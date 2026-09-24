# Authentication test doubles

Generated Moq doubles implement every current repository and driver port. They
need no database, provider credentials, network access, or mocking runtime library
when running tests. The generated code imports only the domain and Go standard library.

| Package | Mocks |
| --- | --- |
| `generated/mocks/repository` | `UserMock`, `IdentityMock`, `TransactionMock`, `TransactionManagerMock`, `ChallengeMock`, `SessionMock`, `PolicyMock` |
| `generated/mocks/gateway` | `MethodMock`, `CallbackMethodMock`, `SessionManagerMock` |

Import these packages as `repositorymock` and `gatewaymock`. Each mock exposes a
function field for each operation, such as `GetFunc`, `VerifyFunc`, or `CreateFunc`.
Configure only the operations a test expects. Calling an unconfigured operation
panics, making unexpected calls visible. `GetCalls()`, `VerifyCalls()`, and similar
accessors record arguments for assertions. `CallbackMethodMock` also implements
all the embedded `Method` operations.

For example, configure a policy lookup failure:

```go
policies := &repositorymock.PolicyMock{
    GetFunc: func(context.Context, entity.TenantID, entity.Intent) (*entity.Policy, error) {
        return nil, repository.ErrNotFound
    },
}
```

Use a fresh mock per test. Call recording is synchronized, but your function
closures, returned objects, and referenced request data are still your test's
responsibility. Supply independent snapshots when simulating storage reads, and
do not mutate captured values concurrently.

## Run the demonstration

```sh
go test -v ./domain/service -run TestAuthenticationWithMocks
```

[`TestAuthenticationWithMocks`](../domain/service/mock_flow_test.go)
loads an authentication transaction and its policy through repository mocks, simulates password and
TOTP verification, evaluates the real `all(primary, mfa)` policy after each proof,
and supplies the verified principal to a session mock. Its output shows:

1. An empty authentication transaction needs primary authentication and MFA.
2. Password proof identifies a user; MFA becomes available.
3. TOTP proof satisfies the remaining policy.
4. A fake session credential is returned.

This older test explicitly assembles the evaluator steps. The real application
and delivery tests now exercise the complete flows, using the generated mocks
plus `internal/testutil/authfixture` for transactional storage and idempotent
session behavior. Run them with:

```sh
go test -race ./application/... ./delivery/...
```

Generated mocks themselves do not provide locking or rollback. The fixture store
checks transaction contexts and root locks, copies snapshots, and rolls back on
failure. Neither it nor the fixture drivers are production implementations. See
[implementation tests](authentication-testing.md).

## Regenerate

Build the generator with Go 1.26 or newer, then run the source directives:

```sh
go install github.com/matryer/moq@v0.7.1
go generate ./domain/repository ./domain/gateway
go test -race ./...
```

The targeted commands regenerate mocks without invoking implgen or requiring
PostgreSQL. Generated doubles are separate from `generated/pb`, so
protobuf generation does not remove them. Do not edit generated mocks by hand.
