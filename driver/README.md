# Driver catalog

Choose implementations by the dependency they satisfy:

| Component | Implementation | Constructor |
| --- | --- | --- |
| Expiring authentication transactions | `transaction/redis` | `New(manager)` |
| Optimistic Redis scopes | `transactionmanager/redis` | `New(client)` |
| Completion receipts | `completion/pgx` | `New(manager)` |
| Authentication transactions | `transaction/pgx` | `New(manager)` |
| Storage transactions | `transactionmanager/pgx` | `New(pool)` |
| Users | `user/pgx` | `New(manager)` |
| Identities | `identity/pgx` | `New(manager)` |
| Durable events | `event/pgx` | `New(ctx, manager)` |
| Redis sessions | `session/redis` | `NewSessionManager(client, tenant, lifetime)` |
| In-memory events | `event/inmem` | `New(manager)` |
| Authentication transactions | `transaction/inmem` | `New(manager)` |
| Storage transactions | `transactionmanager/inmem` | `New()` |
| Users | `user/inmem` | `New(manager)` |
| Identities | `identity/inmem` | `New(manager)` |
| Sessions | `session/inmem` | `NewSessionManager(tenant, lifetime)` |
| Policies | `policy/configured` | `NewImplementation(policies, maxNodes)` |
| Login/password | [`method/logpass`](method/logpass/README.md) | `New(identities, config)` |
| Google OAuth | [`method/goth`](method/goth/README.md) | `New(config)` |
| Authentication methods | `method/mock` | `NewPassword()`, `NewTOTP()` |

```go
manager := transactionmanager.New()
deps := authorization.Dependencies{
    Events: event.New(manager),
    TransactionManager: manager,
    Transactions: transaction.New(manager),
    Users: user.New(manager),
    Identities: identity.New(manager),
    Access: access,
    Eligible: eligible,
}
```

The import aliases above refer to each component's `inmem` package. See
[`examples/main.go`](../examples/main.go) for complete wiring. Sessions are
selected through `WithSessionManager`; methods through policy `Method` nodes.
The constructor builds configured policies itself, so hosts normally do not
construct the policy adapter directly.

All cooperating in-memory repositories must receive the same manager. Data is
process-local and transactions serialize across that manager. `driver/internal`
contains shared storage mechanics, not an additional selectable driver.

`method/mock` uses fixed Alice credentials (`test-password`, `123456`) solely for
demonstrations. It does not implement password hashing or real TOTP. Its typed
`Input` is decoded by the example's delivery adapter. Access and eligibility
remain host callbacks. Challenges are part of the authentication transaction,
so there is no independent challenge driver in this catalog.

See [PostgreSQL and Redis drivers](../docs/postgres-redis-drivers.md) for durable storage, migrations, session recovery, and integration tests.
