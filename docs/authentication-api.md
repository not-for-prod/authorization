# Authentication API contract


The optional service adapter uses the `authorization.v1` protobuf package.
Embedded callers continue to use Go contracts; domain code does not depend on
protobuf. Application lifecycle and retry behavior follow the approved
[event-storming model](event-storming/authentication-method-attempts.md).

The service, requests, responses, and supporting messages are defined together in
[service.proto](../api/authorization/v1/service.proto).
Generated Go messages, gRPC interfaces, HTTP gateway bindings, OpenAPI v2, and the
Clay service descriptor live under `generated/pb/authorization/v1`.
Application commands and delivery handlers are implemented.
See [HTTP/gRPC wiring and tests](authentication-testing.md). A standalone server
is not included.

The corresponding Go use-case interface is
[`application.Authentication`](authentication-use-cases.md). Delivery maps protobuf
requests and responses at that boundary.

## Policy discovery

`GetAuthPolicy` is available at `GET /v1/tenants/{tenant_id}/auth/policy`.
The host's existing access hook authorizes the tenant and establishes caller
context. Results contain `intents`, sorted by name, each with `intent` and
`requirements`. Anonymous callers do not see intents configured with
`RequireAuthenticatedUser`. An authorized caller with no visible intents gets
an empty list. No transaction, user lookup, session, or method-driver call is made.

Requirement nodes contain either `any` children, `all` children, or a `method`
with an optional `provider`. Configured group IDs appear as `id` and apply to
children until overridden by another group ID. Multiple providers for one method
become `any` alternatives with one provider per leaf. An `all` group does not
prescribe a sequence. The response contains no driver settings, secrets, input
schemas, verified evidence, or private challenge data.

The provided HTTP handler and Clay example omit default-valued protobuf fields.
Clients treat absent strings, booleans, and lists as empty, false, and empty lists
respectively. Policy nodes therefore include only populated IDs, groups and methods.

This is configured policy, not current usability. Start a transaction using a
listed intent, then choose from its `availableMethods` after each operation.
Existing transactions retain their policy snapshot if configuration changes.

Adding this RPC is additive on HTTP/gRPC. Custom implementations of the Go
`Authentication` interface must implement the new `GetAuthPolicy` method.

## Operations

Transaction routes begin with `/v1/tenants/{tenant_id}/auth/transactions`.

| RPC | HTTP route suffix | Purpose |
| --- | --- | --- |
| `CreateTransaction` | `POST` at the prefix | Start an authentication transaction for a configured intent. |
| `GetTransaction` | `GET /{transaction_id}` | Read public progress and currently usable selections. |
| `StartMethod` | `POST /{transaction_id}:startMethod` | Start a method; return its attempt ID, optional challenge and redirect URL. |
| `VerifyMethod` | `POST /{transaction_id}:verifyMethod` | Verify a direct proof or challenge response. |
| `CompleteCallback` | `POST /{transaction_id}/challenges/{challenge_id}:completeCallback` | Complete a callback-capable method using host-parsed input. |

Mutation bodies use protobuf JSON. Path fields need not be repeated in the body.
Intent is a closed protobuf enum: `INTENT_SIGN_UP`, `INTENT_SIGN_IN`,
`INTENT_UPDATE`, and `INTENT_DELETE`. Unspecified and unknown values are rejected.
Requirement, method, and provider IDs remain extensible strings. Clients
choose a `selection` from `transaction.availableMethods`. This is a flat list of
currently usable options, not a representation of the policy's `any`/`all` tree.
After each successful operation, clients use the returned progress to choose again.

Create a transaction:

```http
POST /v1/tenants/acme/auth/transactions
Content-Type: application/json

{"intent":"INTENT_SIGN_IN"}
```

Start the method first, including methods without challenges:

```http
POST /v1/tenants/acme/auth/transactions/tx_1:startMethod
Content-Type: application/json

{"selection":{"requirementId":"primary","methodId":"password"},"input":{}}
```

Copy the returned `attemptId` into the verification request:

```http
POST /v1/tenants/acme/auth/transactions/tx_1:verifyMethod
Content-Type: application/json

{
  "attemptId": "COPY_FROM_START_RESPONSE",
  "input": {"username":"alice","password":"example-only"}
}
```

The example assumes that the host's password driver accepts `username` and
`password`. The core contract does not prescribe those driver fields.

## Driver input and callbacks

`input` is a required `google.protobuf.Struct`, represented as an ordinary JSON
object. An empty object is valid for methods that need no input. The delivery
adapter must select the host-provided decoder for the tenant, method, and operation,
validate its payload, and construct the driver's typed `gateway.MethodInput`.
Protobuf types must not cross into domain driver interfaces. Treat exact identifiers
and integers larger than JavaScript's safe integer range as strings in this object.

`VerifyMethod` and `CompleteCallback` require the `attemptId` returned by Start.
`VerifyMethod.challengeId` is omitted for methods without a challenge. If supplied, it must be
nonempty and identify the pending attempt’s eligible challenge.
Verify resolves selection from the stored attempt; its request has no `selection` field.
The callback operation gets its selection from the stored challenge and must match the pending attempt.
An incorrect or stale attempt fails before driver dispatch.

`CompleteCallback` is an engine command, not an OAuth provider redirect endpoint.
The host owns the provider's HTTP callback route, resolves the original authentication transaction,
and supplies parsed callback input. The driver still verifies correlation, state,
and provider proof. Input supplied to this command is not trusted evidence.

## Progress, results, and trust boundaries

`transaction.pendingAttempt` exposes the pending attempt ID, selection, optional
challenge ID and `ready` flag. While pending, `availableMethods` is empty: finish
that attempt before starting another. `transaction.status` identifies active,
finalizing or completed state. Duplicate Start for the same pending selection
returns its stored outcome. Successful Verify retries return stored progress
without verifying again; only the final attempt can recover finalization.

Public transaction and challenge messages exclude user binding, accumulated
evidence, and private challenge state. `policySatisfied` reports only the policy
decision. A successful verification may still require more authentication.
`result` is present only after the application resolves the subject and finalizes
the authentication transaction; its session is present only when that intent issues one. Reading
transaction progress never returns a session token.

Tenant, transaction, and challenge IDs select resources; they are not authorization
credentials. Before exposing handlers, the host must establish access through
trusted request context (for example, a continuation cookie or metadata) and enforce
tenant and authentication transaction ownership. Existing-user intents must bind the user from trusted
authentication context, not a request-supplied user ID. The host Access hook
must resolve a stable caller key; the application enforces its stored association.

Request shape rules use `buf.validate`. Each implemented handler invokes
Protovalidate, covering both gRPC and Clay HTTP as well as direct handler calls. Driver payload validation belongs to the selected
decoder; eligibility, expiration, user binding, and replay protection belong to
the application/domain flow. Never log driver inputs or issued session tokens;
configure request size limits and credential response cache controls at delivery.

There are no persistence CRUD or policy-management endpoints. Session refresh and
revocation have no endpoints. Original completion requests are retryable through
idempotent session issuance; unknown driver outcomes require restarting after
claim expiry. Classified errors map to generic gRPC/HTTP status messages without
internal diagnostics.

## Generation

Use Go 1.26 or newer. Generator executables must also be built with a compatible
Go version. The repository's `make dependency` installs generators but also changes
helper dependencies, so review its scope before invoking it. For an API change:

```sh
buf format -w
make pb
go test ./...
```

`make pb` refreshes Buf dependencies, lints, and regenerates protobuf output;
review buf.lock and generated changes. It does not execute the emitted implgen
directive. The concrete handlers are implemented under
`delivery/api/authentication-service`; do not regenerate stubs over them.
Use `NewAuthenticationServiceServiceDesc(handler)` for host registration, or the
implemented handler's `HTTPHandler(ctx)` convenience for the existing HTTP routes.

The enum replaces the former string intent fields using new field numbers; old
numbers are reserved. This is a client migration: old string payloads are no longer
accepted. SIGN_UP now creates an account from a verified unregistered identity after policy
satisfaction. UPDATE and DELETE business operations remain unimplemented.
