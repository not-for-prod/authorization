# Authentication flows: first runnable implementation

Method lifecycle is superseded by the approved [pending-attempt model](authentication-method-attempts.md): Start is mandatory, Verify/Callback require its attempt ID, and state transitions emit events atomically.

Status: **Approved by the owner on 2026-09-23.**

## Approval scope

The owner approved this event-storming model on 2026-09-23 and asked to keep
current logic. Implement the existing-user flows below and preserve tenant IDs
in the current API routes and application contracts. Registration, identity
linking, and host-derived tenant routing remain future work. This approval also
covers root-owned challenges and the idempotent session issuance contract.

## Scope for first testing

Implement all five existing RPCs through the generated Clay descriptor and the
`domain/application.Authentication` interface. Exercise real delivery and
application code together using HTTP tests with fake repositories and drivers.
The existing hand-assembled policy demo becomes an actual use-case test.

The first slice authenticates **existing users**. Host configuration determines
whether an intent starts anonymously or requires an authenticated user, and
whether completion returns only a principal or also issues a session. Intent
names remain arbitrary strings. Unknown identities fail; signup and identity
linking are not implicitly implemented as authentication.

Actors: client, host delivery adapter, application service, authentication
transaction, method driver, host user/identity storage, host session manager.
Tests use seeded users and fake methods; no live providers or database are needed.

## Happy path

```mermaid
sequenceDiagram
    actor Client
    participant HTTP as Clay HTTP / gRPC
    participant App as Authentication application
    participant Tx as Authentication transaction
    participant Driver as Configured method
    participant Users as User / identity reads
    participant Sessions as Session manager
    Client->>HTTP: CreateTransaction(tenant, intent)
    HTTP->>HTTP: Validate input and establish trusted caller
    HTTP->>App: CreateTransaction
    App->>Tx: Create authentication transaction with fixed policy and expiry
    Tx-->>App: TransactionCreated
    App-->>Client: Progress and available methods
    opt Method needs a challenge
        Client->>HTTP: StartMethod(selection, input)
        HTTP->>App: Authorize access, decode input, StartMethod
        App->>Tx: Lock, claim attempt, commit
        App->>Driver: Start outside storage transaction
        Driver-->>App: Challenge and optional redirect
        App->>Tx: Lock, verify claim, attach challenge, commit
        App-->>Client: Public challenge reference and redirect
    end
    loop Until policy is satisfied
        Client->>HTTP: VerifyMethod or CompleteCallback
        HTTP->>App: Authorized request with typed driver input
        App->>Tx: Lock, check eligibility, claim verification, commit
        App->>Driver: Verify or Callback outside storage transaction
        Driver-->>App: Verified evidence and subject
        App->>Users: Resolve existing user and check eligibility
        App->>Tx: Lock, check claim and subject, accept proof, commit
        Note over Tx: Challenge consumption and evidence commit together
        App->>App: Evaluate policy and remaining method availability
        App-->>Client: Progress when more proof is required
    end
    App->>Tx: Persist final principal and completion intent
    opt Intent issues a session
        App->>Sessions: Create with stable tenant + transaction key
        Sessions-->>App: Session for that key
    end
    App->>Tx: Record completion under root lock
    App-->>Client: Principal and optional session credential
```

`GetTransaction` is read-only: authorize access, read the authentication transaction, evaluate its
fixed policy, and return current progress. It never starts a driver, repairs state,
or returns a session credential. Callback method selection always comes from the
stored challenge; callback payload fields cannot override it.

## Rejection and concurrency

```mermaid
sequenceDiagram
    actor Client
    participant HTTP as Delivery
    participant App as Application
    participant Tx as Authentication transaction
    participant Driver as Method driver
    Client->>HTTP: Start, verify, or callback request
    alt Invalid shape or unauthorized caller
        HTTP-->>Client: Reject before application/driver execution
    else Authorized request
        HTTP->>App: Typed command
        App->>Tx: Lock and check current state
        alt Expired, consumed, wrong selection, or another operation in flight
            App-->>Client: Reject without invoking the driver
        else Eligible attempt
            App->>Tx: Record unique operation claim and commit
            App->>Driver: Invoke outside lock
            alt Definitively invalid proof
                App->>Tx: Record failed attempt and release claim
                App-->>Client: Authentication rejected, no evidence or session
            else Verified proof resolves to another user
                App->>Tx: Reject subject change and retire attempt
                App-->>Client: Subject mismatch, binding stays unchanged
            else Successful proof
                App->>Tx: Recheck claim and expiry under root lock
                App->>Tx: Accept once and consume challenge in the same commit
                App-->>Client: Updated progress or final result
            end
        end
    end
```

User eligibility is supplied by a required host rule because user status values
are not yet defined. Unknown users and identities fail closed. The demo rule
accepts only its explicitly seeded users. Proofs must match tenant, method,
provider, requirement scope, and the transaction's user binding.

## Failure and recovery

```mermaid
sequenceDiagram
    actor Client
    participant App as Application
    participant Tx as Authentication transaction
    participant Driver as Method driver
    participant Sessions as Session manager
    alt Start or verification has an unknown external outcome
        App->>Tx: Persist claim before external call
        App->>Driver: Start, Verify, or Callback
        Note over App,Driver: Timeout or process loss, outcome cannot be recovered generically
        Client->>App: Retry after the operation deadline
        App->>Tx: Lock, retire abandoned authentication transaction, commit
        App-->>Client: Restart authentication with a new transaction
        Note over Tx,Driver: Old claims cannot accept late results or repeat that external call
    else Policy satisfied but session delivery interrupted
        App->>Tx: Persist principal and stable session issuance key
        App->>Sessions: Create(key, principal)
        Sessions-->>App: Session created
        Note over App,Client: Commit or response may be lost
        Client->>App: Repeat completion request with authorized transaction access
        App->>Tx: Read persisted completion intent
        App->>Sessions: Create(same key, same principal)
        Sessions-->>App: Same session and credential
        App->>Tx: Record completion if still pending
        App-->>Client: Same completed result without verifying proof again
    end
```

An in-flight retry returns a conflict until the claim deadline. Abandoned
non-idempotent operations require a fresh authentication transaction; they are never retried blindly.
Definitive rejection may permit another attempt within configured limits.
Cancellation before dispatch releases a claim when possible; cancellation after
dispatch follows the unknown-outcome route. Absolute authentication transaction expiry bounds all
continuation and result retrieval, including session recovery. `GetTransaction`
does not trigger recovery. No background worker is needed for the first test slice.

Session issuance requires a strengthened gateway contract: the same key and
principal return the same session/credential during the authentication transaction's recovery window;
reuse of a key with different principal data is rejected. The gateway owns that
idempotency guarantee, including any backing session persistence. Raw session
tokens are not added to the transaction repository.

## Aggregate boundaries

| Root / boundary | Purpose | Owned state and invariants | Write/lock boundary |
| --- | --- | --- | --- |
| Authentication transaction | Protect one authentication transaction's progress. | Fixed tenant, intent, policy, caller association, expiry, user binding, evidence, bounded challenges, one operation claim, final principal, completion intent. A consumed challenge and its accepted evidence are inseparable. | Existing roots use `Transaction.Get(..., ForUpdate())` inside `TransactionManager.Do`; one transaction repository saves root and owned attempts. |
| User | Represent an existing canonical subject. | Existing user record; no lifecycle changes in this slice. | Read through the user port and apply the host eligibility rule. |
| Identity | Identify the existing owner of an issuer/subject pair. | Existing unique issuer/subject mapping; no creation, reassignment, or linking in this slice. | Read-only resolution through the identity port. |
| Session, behind the host gateway | Represent the credential issued for a completed principal. | Session validity and idempotent issuance; independent of authentication storage. | Session manager call after committing the transaction's completion intent; no shared or distributed lock. |

Transaction creation uses a generated ID with uniqueness enforced by its storage
adapter. Tenant plus transaction ID is the canonical lock identity. Challenges
are owned attempts, even when selected by their own ID; every attempt mutation
goes through its transaction root. Host-configured finite limits bound policy
size, challenge count/state size, attempt count, and transaction/claim lifetime.
There is no unbounded attempt history inside this aggregate.

The transaction stores stable user/identity references and verified facts; it
does not own or mutate user or identity roots. Cross-store revocation cannot be
made atomic with proof acceptance by this interface. User eligibility is checked
before binding and finalization; continuous revocation is outside this first slice.

## Commands and state-change facts

| Entry point | Root commands / facts | Next action |
| --- | --- | --- |
| `CreateTransaction` | Create → `TransactionCreated` | Evaluate initial method availability. |
| `StartMethod` | Claim attempt → `MethodAttemptStarted`; attach challenge → `ChallengeCreated` | Return public challenge/redirect after commit. |
| `VerifyMethod`, `CompleteCallback` | Claim verification; accept → `EvidenceAccepted`; reject → `MethodAttemptRejected` | Re-evaluate policy; finalize only with a bound eligible user. |
| Internal finalization | Prepare → `AuthenticationSatisfied`; complete → `AuthenticationCompleted` | For session-enabled intents, call the session gateway using committed issuance identity. |
| Mutating retry after an abandoned claim | Abandon → `AuthenticationAbandoned` | Client starts a new authentication transaction. |
| `GetTransaction` | None | Read-only progress. |

These facts describe root transitions for the review. They do not introduce
broker publication, consumers, or an outbox. There are no event-driven writes to
User or Identity. The external session action is recoverable from the committed
transaction completion intent and its stable idempotency key; no cross-root
atomic commit is claimed.

## Decisions included in this proposal

1. Support all five API operations for existing users, including direct proofs,
   challenges, callbacks, MFA, and principal-only or session-bearing completion.
   Signup and linking fail as unsupported intent behavior unless separately designed.
2. Own challenges within the authentication transaction's persistence boundary.
   Application code will no longer independently write `repository.Challenge`;
   revise internal storage contracts and regenerate mocks accordingly.
3. Pin a policy snapshot for each authentication transaction so configuration changes cannot rewrite
   an in-progress authentication transaction's proof requirements.
4. Require host-provided access checks, user eligibility, and driver-input decoding.
   Callback decoding cannot choose the execution method; the stored challenge does.
   Tests inject these hooks; the service has no default allow-all authorization.
5. Serialize operations with bounded claims, fail closed on unknown driver outcomes,
   and strengthen session creation with a stable issuance key for completion retries.
6. Deliver an HTTP integration-test path through the real Clay adapter plus
   application tests with generated mocks. This slice does not add a standalone
   public server, live provider drivers, Redis, or PostgreSQL.

## Approval

The six decisions, diagrams, and root boundaries above are approved. Implement
without changing the public `authorization.v1` RPC names, routes, or response shapes.
