# Testing authentication flows


The application and delivery layers implement the approved existing-user scope.
Use Go 1.26 or newer, with generators built using a compatible toolchain.

```sh
go test -race ./domain/...
go test -race ./application/...
go test -race ./delivery/...
go test -race ./...
```

The application tests cover password/MFA, challenged verification, callbacks,
caller/tenant isolation, expiry, wrong subjects, unknown identities, concurrent
attempts, rollback, cancellation, and session recovery. Delivery tests send HTTP
requests through the real generated Clay routes and the real application service.
They check all six operations, validation, response privacy, and safe errors.
No live provider, database, listener, or credentials are required.

`internal/testutil/authfixture` combines strict generated driver mocks with a
stateful test store. The store copies snapshots, enforces transaction contexts
and root locks, rolls back on error, and permits deterministic persistence
failures. It is test support, not a deployable persistence adapter. Its session
fixture preserves a credential by issuance key so response/commit loss can be tested.
Concrete host adapters still need their own storage/concurrency integration tests.

## Run the local example

`go run ./examples` starts the mock Clay host with HTTP/gRPC on port `8080`
and Swagger UI at `http://localhost:8080/docs/`. The pinned Clay server listens
on all interfaces.
See [the example walkthrough](../examples/README.md) for credentials, cookie-jar
handling, and password/MFA requests. The embedded and delivery test suites cover HTTP flows without opening a listener. This is local test wiring, not a
production storage/provider implementation.

## Wiring delivery in a host

```go
import auth "authorization"

service, err := auth.New(dependencies, options...)
if err != nil {
    return err
}

httpHandler, err := auth.NewHTTPHandler(ctx, service, decodeDriverInput)
if err != nil {
    return err
}
// Mount httpHandler behind the host's trusted caller-context middleware.
```

`NewHTTPHandler` registers existing `/v1/tenants/{tenant_id}/auth/transactions`
routes, limits HTTP request bodies to 1 MiB, and sets `Cache-Control: no-store`.
Each concrete handler runs Protovalidate, so direct, gRPC, and Clay HTTP calls
cannot skip request validation. The host decoder returns driver-owned typed
input. For callbacks, decoding waits until the application resolves the stored
challenge selection; the callback payload cannot choose the driver.

There is no production standalone server or `cmd` composition tree; `examples/main.go`
is a local test host.
Host authentication, callback correlation, provider credentials, storage, and
session adapters remain injected dependencies. The public `authorization` package supports embedding from other Go modules.
The generated gRPC bindings remain internal; the public wrapper currently exposes
HTTP and direct Go calls.

## Retries

Every method requires Start first. Retain its `attemptId` for Verify/Callback,
including direct password/TOTP methods. Duplicate Start reuses the pending result;
a successful Verify replay does not dispatch proof verification again.
Definitively rejected proofs may be retried with the same attempt ID within limits. An unknown
method outcome keeps its claim. Competing calls are rejected while the claim is
live; after the deadline a mutating retry abandons that authentication transaction
and the client must create another. Late results cannot be accepted.

Session completion is different: the application persists the principal before
calling the session gateway with `CreateSessionRequest`. A repeated original
completion operation uses the same tenant, transaction key, principal and
recovery deadline. The gateway must return the same credential. Retrying another
method is not a completion retry. All recovery is bounded by transaction expiry.
`GetTransaction` never returns credentials or performs recovery writes.


## Public constructor tests

```sh
go test -v ./examples/embedded
go test -run TestExternalHost -v .
```

The example defines its own driver, typed proof input, storage doubles, access
hook, and policy-bound nested options using only public imports. It exercises
proof rejection, successful completion, progress reads, and the real Clay HTTP
routes. Its single-threaded storage doubles are test-only; use the stricter
`internal/testutil/authfixture` suite for the engine's transactional guarantees.

Root package tests cover constructor failures, tenant-specific driver bindings,
shared drivers across intents, immutable policy inputs, policy bounds, required
users, and explicit session issuance. No JWT adapter or default issuer is installed.
