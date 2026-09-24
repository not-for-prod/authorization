# Authentication application contracts


[`application.Authentication`](../domain/application/authentication.go) defines
the six use cases for application implementations. Its requests and responses
are plain Go values, independent of HTTP, protobuf, storage adapters, and provider
payload formats. The existing `service.Evaluator` remains the pure policy evaluator.

| Method | Contract |
| --- | --- |
| `GetAuthPolicy` | List caller-visible intents and independent configured policy snapshots, without driver calls or writes. |
| `CreateTransaction` | Start a tenant/intent authentication transaction and return its initial progress. |
| `GetTransaction` | Read progress and available methods without writing state or issuing a session. |
| `StartMethod` | Start an eligible method and persist its pending attempt and optional challenge before returning an attempt ID. |
| `VerifyMethod` | Verify a direct or challenged proof, accept evidence, and return progress with an optional completed result. |
| `CompleteCallback` | Verify callback proof against the stored selection and follow the same evidence-acceptance rules. |

The interface and shared progress/result types live in `authentication.go`.
Each operation's request and response types live together in its corresponding
method file. `ChallengeReference` lives with `StartMethod`, which returns it.
The implementation lives in
`application/service/authentication-service`. See
[configuration](authentication-configuration.md) for its constructor.

## Inputs and trust

Delivery maps validated protobuf fields into these requests and decodes `input`
into the selected driver's `gateway.MethodInput`. An embedded caller supplies that
typed input directly. Neither path can submit trusted `Evidence`, choose the
callback's selection, or provide private challenge state through these contracts.

`CreateTransactionRequest.AuthenticatedUserID` is optional trusted host input for
existing-user authentication transactions. The application derives the trusted user from the host Access hook, never
from a client-supplied field; an explicit request value must match that user. Its presence does not prove the new intent's policy
has been satisfied. Nil leaves subject identification to method verification.

Tenant and transaction IDs select resources; they do not establish access.
Every caller must pass the host's authentication/continuation authorization boundary.
The required host `Access` hook establishes a stable caller key. The application
persists that association and enforces tenant binding, selection eligibility,
expiry, and subject consistency on every operation.
Direct callers require their own input validation; protobuf validation only covers
callers that actually pass through delivery middleware.

`VerifyMethodRequest.AttemptID` and `CompleteCallbackRequest.AttemptID` are required
references from Start. Start is mandatory even without a challenge.
`VerifyMethodRequest.ChallengeID == nil` means verification without a challenge. A non-nil ID
must be nonempty and match the stored attempt. Verify resolves selection from
that attempt; `VerifyMethodRequest` has no `Selection` field. Callback
completion always requires a challenge and loads its selection from stored state.
Driver input, including passwords or callback credentials, must not be logged.

## Results and implementation boundary

All successful calls return a non-nil response. Responses are usable only when
the error is nil. An error does not establish that an external operation had no
effect and does not authorize blindly retrying it.

`TransactionProgress` excludes user binding and evidence. Its method list contains
currently eligible, available selections; it does not encode `any`/`all` grouping.
`ChallengeReference` excludes private challenge state. `AuthenticationResult` is
present only after subject resolution and finalization, and carries a principal
plus an optional issued session. `PolicySatisfied` alone does not imply completion.

The result reuses domain principal/session types internally. Delivery must map
them explicitly to protobuf, omitting domain evidence and exposing the token only
through the intended successful issuance response. Progress reads cannot return
a completed result or session credential.

The aggregate guards the pending attempt, accepts evidence, evaluates the resulting
policy and emits before/after events. The application commits root state and events
in the same storage transaction. Finalization remains synchronous in Verify/Callback.

The implementation atomically persists transaction-owned challenges and evidence,
uses durable operation claims around driver calls, and recovers session issuance
through a stable key. User creation/linking remain outside this scope.

Verify and callback delivery supply `DecodeInput`, a closure that receives the stored
selection before constructing typed input. Embedded callers may instead supply
`Input` directly. Exactly one is required. The callback never selects a method
from raw provider fields. See [tests and retry behavior](authentication-testing.md).

## Shared proof binding

Verify and Callback use the same intent-independent subject resolution and
`Transaction.AcceptMethodProof` transition. The transaction retains the verified
identity, binds an existing UserID when resolved, and evaluates accumulated evidence.
Unresolved identities are accepted for either intent until finalization. Proofs must
attest the same unresolved identity or the same resolved user.

Finalization requires an existing eligible user for SIGN_IN. SIGN_UP rejects
registered subjects and creates an eligible user and identity only after policy
satisfaction, retaining the existing atomic creation boundary.
