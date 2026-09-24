# Go Auth Engine — Design Context


## Goal

Build a configurable authentication engine as a Go package. The package exposes
contracts and a constructor through which a host application supplies configured
authentication methods, policies, and supporting dependencies.

The host owns provider setup and credentials. It can embed the engine directly
or expose it through its own standalone authentication service. Focus first on
the core authentication domain; transport and deployment belong to the host.

An optional `authorization.v1` protobuf contract and generated Clay/gRPC/HTTP
bindings are defined in `api/authorization/v1`. Application orchestration and
Clay HTTP/gRPC delivery are implemented for existing users. The host supplies
storage, methods, access/eligibility rules, input decoding, and session management.
A tenant-configured [Goth Google driver](../../driver/method/goth/README.md) implements
`oauth/google`. Verify commits an exclusive claim before provider I/O.
CompleteCallback uses the simplified flow: read and validate, call the provider,
then lock and recheck the attempt to accept proof or record definitive rejection.
It does not persist a lease before provider I/O or abandon uncertain callbacks.
Concurrent callbacks may reach the provider; acceptance is serialized by the final
root lock. Callback crash recovery is deferred. Its claim-based domain transitions
run locally within the final persistence transaction and leave no stored claim.

Tenant IDs remain in the current routes. See [API contract](../authentication-api.md)
and [wiring and tests](../authentication-testing.md). The public `authorization.New(dependencies, options...)` constructor supports
other Go modules, and `NewHTTPHandler` wires optional HTTP delivery. No standalone
production server is included; `examples/main.go` is a local mock HTTP test host.

Domain-facing application use cases are defined by `application.Authentication` under
`domain/application`. See [Authentication application contracts](../authentication-use-cases.md)
for typed inputs, progress/results, and the implemented application boundary.

Repository and gateway test doubles live in `generated/mocks`. See
[Authentication test doubles](../authentication-mocks.md) for regeneration and
the evaluator demonstration. Application and delivery integration tests also use
a stateful test fixture for root locking, rollback, and idempotent issuance.
Mocks and fixtures are not production adapters. Optional process-local drivers
are cataloged by dependency under `driver/transaction`, `driver/user`,
`driver/identity`, `driver/transactionmanager`, `driver/event`, and `driver/session`; demo method
drivers live under `driver/method/mock`. The production-facing
[`driver/method/logpass`](../../driver/method/logpass/README.md) verifies SIGN_IN
against identity-owned optional `PasswordHash` values using a default bcrypt
verifier or a host-configured verifier. Issuer defaults to `local`, and the driver
generates a dummy hash for its default bcrypt cost. Credential provisioning remains
host-owned. Public library
packages live at the root; only test support remains under `internal/testutil`.

The `driver/method/telegram` adapter implements official Telegram OIDC as
`oauth/telegram`, with PKCE, RS256 verification and host-owned callback routing.
See [Telegram OAuth driver](../telegram-oauth.md) for wiring and identity semantics.

The approved [pending-attempt lifecycle](../event-storming/authentication-method-attempts.md)
is implemented. Start is mandatory for every method and returns an attempt ID;
Verify and Callback must match that attempt. The transaction owns pending-state
guards, evidence acceptance and policy evaluation. Events record committed user
actions; finalization stays synchronous. `Dependencies.Events` must share the
root storage transaction. The in-memory event driver is atomic but non-durable;
production hosts must provide a durable outbox adapter. See
[domain event integration](../domain-events.md).

Supported authentication capabilities should eventually include:

* username/password
* email/password
* email verification code
* OAuth2/OIDC providers such as Google/GitHub
* Telegram
* Telegram Mini App
* TOTP
* Telegram verification code
* email verification code
* JWT sessions
* Redis sessions
* multi-tenant configuration

---

# Core Model

Authentication is modeled around:

```text
User
Identity

Intent
Policy
Transaction
Requirement
Method
Challenge
Evidence
Principal
Session
```

Core flow:

```text
Tenant + Intent
      ↓
Policy
      ↓
Transaction
      ↓
Evaluate policy
      ↓
Available authentication methods
      ↓
Method.Start
      ↓
optional Challenge
      ↓
Method.Verify
      ↓
resolve Identity / User when needed
      ↓
bind UserID to Transaction
      ↓
Evidence
      ↓
add Evidence to Transaction
      ↓
Evaluate policy again
      ↓
policy satisfied
      ↓
Principal
      ↓
SessionManager
      ↓
Session
```

---

# User

`User` is the canonical internal subject.

```go
type UserID string
type UserStatus string

type User struct {
    ID        UserID
    Status    UserStatus
    CreatedAt time.Time
}
```

The allowed `UserStatus` values and guarded lifecycle transitions remain to be
defined. The current model only declares the status type.

The authentication system should never use email, username, Google ID, Telegram ID, etc. as the canonical user identifier.

Everything ultimately resolves to:

```text
UserID
```

Other services should primarily care about `UserID`, not how the user authenticated.

---

# Identity

An `Identity` connects an external or local identifier to a `User`. Both models
and their supporting ID and kind/status types live together in
`domain/entity/user.go`.
The allowed `IdentityKind` values remain to be defined.

```go
type IdentityID string
type IdentityKind string

type Identity struct {
    ID     IdentityID
    UserID UserID

    Kind    IdentityKind
    Issuer  string
    Subject string

    PasswordHash string // optional host-provisioned encoded password hash

    CreatedAt time.Time
}
```

Examples:

```text
User: user_123

Identities:

  issuer: local
  subject: john@example.com

  issuer: local
  subject: john

  issuer: https://accounts.google.com
  subject: 109238409823409

  issuer: telegram
  subject: 928374928
```

The important uniqueness constraint is:

```text
(issuer, subject) UNIQUE
```

This means the same external identity cannot belong to two users. This is a
persistence invariant to enforce when repositories and storage are introduced;
the model structs alone do not enforce uniqueness.

Conceptually:

```text
user_123
 ├── email: john@example.com
 ├── username: john
 ├── google: 109238409823409
 └── telegram: 928374928
```

This enables account linking naturally.

A user can authenticate through any linked identity, but the resulting authenticated subject is always:

```text
user_123
```

Do not implicitly merge users merely because two external providers report the same email address.

Provider identity should be resolved using stable provider identity data such as:

```text
issuer + subject
```

---

# Intent

An intent describes what authentication is being performed for.

It is a closed lifecycle enum: `sign_in`, `sign_up`, `update`, and `delete`.
Update includes reset and identity changes. Method-specific operations are not intents.
Existing-user authentication and first-version SIGN_UP are implemented. SIGN_UP
collects one verified identity, creates an active user only after the full policy
is satisfied, and rejects identity conflicts. Password enrollment, UPDATE and
DELETE business flows remain to be designed. See [sign-up flow](../event-storming/sign-up.md).

```go
type Intent string
```

Examples:

```text
sign_in
sign_up
update
delete
```

Policy lookup is based on:

```text
TenantID + Intent
```

---

# Tenant

Authentication configuration and policies are tenant-specific.

```go
type TenantID string
```

Every transaction is permanently bound to one tenant.

```go
type Transaction struct {
ID       TransactionID
TenantID TenantID
Intent   Intent
UserID   *UserID
Evidence []Evidence
}
```

Tenant must never be changed after transaction creation.

---

# Policy

Policy defines the valid authentication flow.

It does NOT define one hardcoded sequential pipeline.

Policy configuration uses `any` for at least one matching child and `all` for
every child. These map to `AnyOfRequirement` and `AllOfRequirement` in Go.

Example:

```yaml
all:

  - id: primary
    any:

      - method: password

      - method: email_code

      - method: oauth
        providers:
          - google
          - github

  - id: mfa
    any:

      - method: totp

      - method: telegram_code
```

Meaning:

```text
(password OR email_code OR oauth/google OR oauth/github)
AND
(totp OR telegram_code)
```

The actual runtime flow depends on:

```text
Policy
+ existing Evidence
+ method availability
+ user choice
```

---

# Requirement Tree

Policy is represented as an AST.

```go
type Requirement interface {
    requirement()
}
```

Supported nodes initially:

```go
type MethodRequirement struct {
    Method    MethodID
    Providers []ProviderID
}

type AnyOfRequirement struct {
    ID       RequirementID
    Children []Requirement
}

type AllOfRequirement struct {
    ID       RequirementID
    Children []Requirement
}
```

Requirement IDs such as:

```text
primary
mfa
```

are configurable.

The engine should NOT assign special semantics to those names.

---

# Evidence

Evidence represents something that has already been successfully verified.

```go
type Evidence struct {
    Requirement RequirementID

    Method   MethodID
    Provider ProviderID

    VerifiedAt time.Time
}
```

Example:

```go
Evidence{
    Requirement: "primary",
    Method:      "oauth",
    Provider:    "google",
}
```

Another:

```go
Evidence{
    Requirement: "mfa",
    Method:      "totp",
}
```

Evidence must be associated with the requirement it was used to satisfy.

This prevents one proof from accidentally satisfying multiple stages.

For example:

```text
email_code used for primary
```

must not automatically also satisfy:

```text
email_code allowed for mfa
```

---

# Policy Evaluation

Evaluator compares:

```text
Transaction Evidence
        against
Policy Requirements
```

The pure evaluator is implemented by the zero-value
`domain/service.Evaluator`. It matches the following interface shape.
See [Policy evaluation](../policy-evaluation.md) for scope inheritance,
validation, simplification, and error semantics.

```go
type Evaluator interface {
    Evaluate(
        ctx context.Context,
        tx Transaction,
        policy Policy,
    ) (Decision, error)
}
```

Decision:

```go
type Decision struct {
    Satisfied bool
    Remaining Requirement
}
```

`Remaining` is the simplified unsatisfied policy tree.

Example:

Original:

```text
all(
    any(password, email_code),
    any(totp, telegram_code),
)
```

Evidence:

```text
email_code -> primary
```

Remaining:

```text
any(totp, telegram_code)
```

When fully satisfied:

```go
Decision{
    Satisfied: true,
    Remaining: nil,
}
```

---

# Authentication Method

A Method represents an authentication mechanism.

Examples:

```text
password
email_code
oauth
totp
telegram_code
telegram_mini_app
```

```go
type MethodID string
```

OAuth is considered one method.

Google and GitHub are providers of that method:

```text
method   = oauth
provider = google
```

or:

```text
method   = oauth
provider = github
```

---

# Provider

```go
type ProviderID string
```

Provider is optional.

Examples:

```text
oauth/google
oauth/github
```

Methods such as:

```text
password
totp
email_code
```

usually have no provider.

---

# Method Interface

The engine-facing contracts live in `domain/gateway`. Infrastructure
drivers implement them using host-configured dependencies. See
[Authentication driver contracts](../authentication-drivers.md) for the input,
result, and callback contracts. The types in this sketch omit package qualifiers.

```go
type Method interface {
    Name() MethodID

    Available(
        ctx context.Context,
        tx Transaction,
        selection MethodSelection,
    ) (bool, error)

    Start(
        ctx context.Context,
        tx Transaction,
        selection MethodSelection,
        input MethodInput,
    ) (StartResult, error)

    Verify(
        ctx context.Context,
        tx Transaction,
        selection MethodSelection,
        challenge *Challenge,
        input MethodInput,
    ) (VerificationResult, error)
}
```

`VerificationResult` carries `Evidence` plus exactly one verified subject:
`Identity` (kind, issuer, subject) for resolution, or `UserID` when the driver
has verified an existing user's credential. The engine owns user binding.
`StartResult` carries an optional private challenge and a public redirect URL.
`MethodInput` is implemented by driver-owned typed inputs through the exported
`AuthenticationInput()` marker; drivers validate the concrete type and fields.

Selection describes what the user is currently trying to satisfy:

```go
type MethodSelection struct {
    Requirement RequirementID
    Method      MethodID
    Provider    ProviderID
}
```

Example:

```go
MethodSelection{
    Requirement: "primary",
    Method:      "oauth",
    Provider:    "google",
}
```

---

# Method Availability

Policy determines which methods are valid.

`Method.Available()` determines whether they can actually be used right now.

Example before a user has been identified:

```text
email_code    available
password      available

totp          unavailable
telegram_code unavailable
```

After primary authentication identifies the user:

```text
totp available if user has TOTP enrollment

telegram_code available if user has a Telegram identity linked
```

Runtime selection therefore works as:

```text
Decision.Remaining
      ↓
extract possible methods
      ↓
Method.Available()
      ↓
frontend receives usable options
```

---

# Challenge

Challenge represents an unfinished attempt to verify one method.

```go
type Challenge struct {
    ID            ChallengeID
    TransactionID TransactionID

    Selection MethodSelection

    Status ChallengeStatus

    CreatedAt time.Time
    ExpiresAt time.Time

    State []byte
}
```

Examples:

Email code challenge may contain private state such as:

```text
email
hashed code
attempt count
```

OAuth challenge may contain:

```text
state
PKCE verifier
redirect URI
```

Conceptual distinction:

```text
Transaction = entire authentication transaction

Challenge = optional private state associated with a pending method attempt

Evidence = completed proof
```

Not every method requires a challenge.

Examples:

```text
password → may verify directly
totp     → may verify directly

email_code → usually requires challenge
oauth      → requires challenge
```

---

# OAuth Callback Handling

A provider callback completes method verification. Drivers that need a distinct
callback entry point implement the optional `CallbackMethod` interface in addition
to `Method`. Password and other direct methods need only implement `Method`.

```go
type CallbackMethod interface {
    Method

    Callback(
        ctx context.Context,
        tx Transaction,
        selection MethodSelection,
        challenge Challenge,
        input MethodInput,
    ) (VerificationResult, error)
}
```

Flow:

```text
Method.Start
    ↓
engine persists challenge
    ↓
host redirects to provider
    ↓
provider callback reaches host route
    ↓
host parses driver-specific input
    ↓
engine loads original transaction/challenge and checks eligibility
    ↓
CallbackMethod.Callback verifies correlation and provider proof
    ↓
VerificationResult
    ↓
engine resolves subject, binds user, and records Evidence
```

`Callback` and `Verify` produce the same kind of result. A callback is an
alternative verification entry point, not an extra proof or policy stage.
The engine must prevent replay and duplicate challenge completion. Drivers
must not mutate engine state, bind users, evaluate policies, or create sessions.
HTTP route registration and response writing remain host responsibilities.

---

# Host Configuration

The host application constructs its authentication methods and passes them to
our package's authentication service constructor. Methods implement the domain
contract; provider-specific settings stay inside those implementations.

For example, the host configures an OAuth method with Google and GitHub, using
its own credentials and chosen OAuth library. Policies still select:

```text
method   = oauth
provider = google
```

Method registration and lookup must respect the transaction's tenant. Two
tenants may register different implementations or credentials for the same
method ID without sharing configuration accidentally.

The host also supplies session management and storage dependencies through Go.
The policy document contains only tenant/intent requirements using `any` and
`all`; it does not configure drivers, secrets, provider clients, or sessions.
The same domain policy trees can be constructed directly in Go. YAML is an
optional representation, not a requirement of the core engine.

Do not introduce a `MethodDriver` factory or a driver registry in the core.
Storage ports are defined under `domain/repository`; the host supplies
their adapters. See [Authentication storage contracts](../authentication-storage.md).
The public constructor accepts nested tenant/intent options and policies built
with `All`, `Any`, and `Method(driver, ...)`. Policy leaves bind concrete host
drivers directly; construction derives tenant bindings and immutable pure policy
trees. There is no default session implementation: session issuance requires an
explicit session manager. The YAML policy-loading helper remains future work. See
[Authentication engine configuration](../authentication-configuration.md) for
the constructor and the tenant policy document.

---

# Transaction

Transaction contains current authentication state. The following sketch shows
its core fields; the implementation also owns caller association, a policy and
settings snapshot, bounded challenges, an operation claim, lifecycle status,
final principal, and completion intent. See `domain/entity/transaction.go`.
All changes to an existing root require its storage lock. Driver calls run outside
that lock; successful evidence and challenge consumption persist atomically.

```go
type Transaction struct {
    ID       TransactionID
    TenantID TenantID
    Intent   Intent

    UserID *UserID

    Evidence []Evidence

    CreatedAt time.Time
    ExpiresAt time.Time
}
```

Transaction responsibilities:

```text
hold state
hold accumulated evidence
bind resolved user
```

Transaction.AcceptMethodProof invokes the pure `Evaluator` after shared subject binding
and evidence acceptance. Intent-specific behavior belongs to finalization.

---

# User Resolution

A method may resolve or confirm an identity while it verifies authentication.

Example:

```text
oauth/google verified
      ↓
issuer + subject extracted
      ↓
IdentityRepository lookup
      ↓
Identity.UserID
      ↓
Transaction.AcceptEvidence binds UserID
```

Similarly:

```text
email code verified
      ↓
email identity resolved
      ↓
UserID bound to transaction
```

Methods prove authentication.

Identity resolution determines which internal `User` that proof belongs to.
For both SIGN_IN and SIGN_UP, the application resolves `VerificationResult` through
existing identity/user records when available. The transaction retains a private
verified identity and binds its existing UserID when resolved. Unresolved identities
can collect evidence without a UserID. At policy satisfaction, SIGN_IN requires an
existing eligible user; SIGN_UP requires an unregistered identity and creates User
and Identity atomically with final binding and event publication. Linking is not implemented. Concrete
storage adapters remain host responsibilities.

---

# Principal

After policy is satisfied, transaction is finalized into a Principal.

```go
type Principal struct {
    TenantID TenantID
    UserID   UserID

    Evidence []Evidence

    AuthenticatedAt time.Time
}
```

Flow:

```text
Transaction
    ↓ policy satisfied
Principal
    ↓
SessionManager
```

Principal avoids coupling sessions directly to authentication transactions.

---

# Sessions

The session management contract is defined as
`domain/gateway.SessionManager`. Logical session storage is a separate
`domain/repository.Session` port.

Session creation happens only after policy satisfaction.

Methods must NEVER create sessions.

```go
type SessionManager interface {
    Create(
        ctx context.Context,
        req CreateSessionRequest,
    ) (IssuedSession, error)

    Validate(
        ctx context.Context,
        token string,
    ) (Session, error)

    Revoke(
        ctx context.Context,
        sessionID SessionID,
    ) error
}
```

`CreateSessionRequest` carries a transaction key, final principal, and recovery
deadline. The tenant's session manager must return the same session/credential
for repeated calls with that key and principal, including concurrent calls.
Different principal data for the same key is rejected. The application persists
completion intent before issuance and never stores the raw token on Transaction.

Host implementations may include:

```text
JWTSessionManager
RedisSessionManager
```

Logical session:

```go
type Session struct {
    ID SessionID

    Principal Principal

    IssuedAt  time.Time
    ExpiresAt time.Time
}
```

Credential returned to the caller:

```go
type IssuedSession struct {
    Session Session
    Token   string
}
```

For JWT:

```text
Token = JWT
```

For Redis:

```text
Token = opaque random session token
```

---

# Engine Responsibility

Engine is the orchestrator.

Conceptually:

```text
Create transaction

↓

load Policy using:
tenant + intent

↓

evaluate

↓

determine remaining requirements

↓

filter available methods

↓

user selects method/provider

↓

Start method

↓

Verify method

↓

resolve Identity / User when needed

↓

bind UserID to Transaction

↓

receive Evidence

↓

add Evidence to Transaction

↓

evaluate again

↓

repeat until satisfied

↓

create Principal

↓

SessionManager.Create
```

The engine should not implement:

```text
password verification
OAuth
TOTP
email sending
Telegram API
JWT internals
Redis internals
```

Those belong behind their respective interfaces.

---

# Important Design Rules

1. `UserID` is the canonical internal identity.

2. Email, username, Google subject, Telegram ID, etc. are `Identity` records linked to a User.

3. External identities are resolved primarily by `issuer + subject`.

4. Do not automatically merge users based solely on equal email addresses.

5. Policy defines the valid authentication flow space.

6. There is no separate hardcoded authentication pipeline.

7. Intent is a closed account lifecycle enum; proof policies remain configurable.

8. Requirement IDs are extensible.

9. Method represents the authentication mechanism.

10. Provider optionally specializes a method.

11. The host supplies configured methods through the constructor; implementation drivers stay outside the core.

12. Methods produce Evidence.

13. Evidence is associated with the Requirement it satisfied.

14. Transaction owns current authentication state.

15. Policy evaluation is separate from Transaction.

16. Challenge represents temporary unfinished verification.

17. Sessions are created only after policy satisfaction.

18. Authentication methods never create sessions themselves.

19. Do not add abstractions until concrete implementation requires them.

---

# Suggested First Coding Task

Implement the pure domain core first:

```text
User
Identity

Intent
TenantID

Requirement
MethodRequirement
AnyOfRequirement
AllOfRequirement

Policy

Evidence

Transaction

Decision
Evaluator
```

Storage interfaces have been added at the owner's request. Concrete persistence
adapters remain host-supplied. The existing-user application/delivery flows have been restored. A first SIGN_UP
implementation is available for owner correction; linking remains outside this slice.

Then write table-driven tests for policy evaluation and identity resolution.

Important evaluation cases:

```text
password satisfies primary

google OAuth satisfies oauth/google

github rejected if only google allowed

primary satisfied but MFA remains

primary + TOTP satisfies full policy

email_code used for primary does not automatically satisfy MFA
```

Important identity cases:

```text
google issuer + subject resolves UserID

telegram identity resolves UserID

same external identity cannot belong to multiple users

same User can have email + google + telegram identities

matching email from OAuth must not automatically merge accounts
```

Do not start with HTTP, Redis, JWT, Goth, Telegram integration, or frontend rendering before the domain and evaluator are stable.
