# Authentication driver contracts

Drivers implement [Method](../domain/gateway/method.go), the engine-facing
port for a configured authentication mechanism. Redirect-based drivers can also
implement [CallbackMethod](../domain/gateway/callback_method.go).
Application orchestration is implemented. Production provider drivers remain
host-supplied; tests use generated mocks.

| Operation | Responsibility |
| --- | --- |
| `Name` | Identify the method, such as `password` or `oauth`. |
| `Available` | Check whether the selection can be used for the current transaction without starting an attempt. |
| `Start` | Initiate verification and return an optional private challenge and public redirect URL. |
| `Verify` | Verify supplied credentials or proof and return evidence plus a verified subject. |
| `Callback` (optional) | Complete verification from a provider callback using the original challenge. |

The host constructs each driver with its provider clients, credentials, and
other dependencies. The engine selects that tenant's registered method instance.
Driver configuration remains in Go; policy YAML still contains only `any`/`all`
requirements referring to method and provider IDs.

## Driver-owned input

Each driver defines the concrete input types it accepts. They implement the
exported `AuthenticationInput()` marker in `MethodInput`, allowing callers and
drivers to share typed inputs without a universal credentials map or raw HTTP
request in the engine contract. The marker only identifies an input value; it
does not establish that the contents are valid or trusted.

For example, a password adapter can define this type in its own package:

```go
type VerifyInput struct {
    Identifier string
    Password   string
}

func (VerifyInput) AuthenticationInput() {}
```

The driver checks the concrete input type and validates its fields. Callback
inputs are likewise defined by their driver and constructed by the host's
transport adapter. Secrets must not be copied into evidence or logs.

## Verification and user resolution

`Verify` and `Callback` return the same `VerificationResult`. It contains evidence
scoped to the selected requirement, method, and provider, plus exactly one of:

- `Identity`: verified kind, issuer, and subject for the engine to resolve.
- `UserID`: a canonical user whose credential the driver actually verified.

The engine validates the result and performs identity lookup, user binding,
evidence accumulation, and policy evaluation. A driver must not return a user ID
merely because the caller supplied it. Identity proof alone does not authorize
account creation or linking. Those remain explicit application use cases.

Methods treat transaction and challenge arguments as read-only, including their
nested slices and pointers. State changes, persistence, and sessions belong to
the engine and its host-provided dependencies.

## Callback completion

```text
Method.Start
    → engine persists challenge
    → host redirects caller
    → provider returns to host callback route
    → host parses driver-specific input
    → engine loads the original challenge and transaction
    → engine dispatches to CallbackMethod.Callback
    → same verified-subject/evidence processing as Method.Verify
```

Callback is an alternative entry point into method verification, not a new
authentication stage. The engine invokes one completion entry point for an
attempt, rather than verifying the same callback twice. Methods without a
callback do not need a dummy callback implementation.

Before dispatch, the engine must check the original transaction/challenge
association, selected method/provider, expiry, and consumption state. The driver
then validates the provider protocol's callback correlation and proof against
the stored private challenge state. The engine must serialize completion and
consume successful challenges so replay cannot produce new evidence. The application and domain now enforce these checks.
A driver must still validate provider-specific proof and correlation.

The domain contracts remain under `domain/gateway`. Public aliases in
`authorization` expose these contracts and their model types to external hosts.
The public constructor is `authorization.New(dependencies, options...)`. Policies
bind `AuthenticationMethod` implementations through `Method(driver, ...)`; see
[constructor configuration](authentication-configuration.md).

## Challenges, failures, and session issuance

A returned challenge must carry a unique ID, the supplied transaction ID and
selection, `ChallengePending`, and creation/expiry times within the transaction.
Private state must fit the configured size limit. The engine copies and persists
it before returning the public reference. Successful consumption clears its state.

Return `gateway.ErrProofRejected` only for a definitive rejection. Other errors
from Start/Verify/Callback are treated as unknown outcomes: the claim remains,
and expiry requires a new authentication transaction instead of blind redispatch.
Honor context cancellation and return sanitized errors without input or secrets.
`Available` and input decoders must not initiate external side effects.

`SessionManager.Create` accepts `CreateSessionRequest{Key, Principal, RecoverUntil}`.
Within the tenant, concurrent/repeated calls with that key must return the same
session and credential until the recovery deadline. Reject changed principal
data and expired issuance requests. The engine verifies the returned principal
and session validity, and stores no token on the authentication transaction.

## Telegram OAuth

The production `driver/method/telegram` adapter implements the official OIDC code
flow as `oauth/telegram`. See [configuration, callback association and recovery](telegram-oauth.md).
