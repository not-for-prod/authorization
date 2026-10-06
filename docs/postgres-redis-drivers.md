# PostgreSQL and Redis drivers

The pgx drivers implement every storage dependency of the engine. Construct one
transaction manager for a host-owned `*pgxpool.Pool` and pass it to all four
adapters. Redis sessions are configured separately for each tenant.

```go
import (
    "context"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/redis/go-redis/v9"

    "github.com/not-for-prod/authorization"
    eventpgx "github.com/not-for-prod/authorization/driver/event/pgx"
    identitypgx "github.com/not-for-prod/authorization/driver/identity/pgx"
    sessionredis "github.com/not-for-prod/authorization/driver/session/redis"
    transactionpgx "github.com/not-for-prod/authorization/driver/transaction/pgx"
    managerpgx "github.com/not-for-prod/authorization/driver/transactionmanager/pgx"
    userpgx "github.com/not-for-prod/authorization/driver/user/pgx"
)

// pool and redisClient are configured and closed by the host.
manager, err := managerpgx.New(pool)
if err != nil { return err }
events, err := eventpgx.New(ctx, manager) // Migrations must already be applied.
if err != nil { return err }
sessions, err := sessionredis.NewSessionManager(redisClient, "tenant", 24*time.Hour)
if err != nil { return err }

deps := authorization.Dependencies{
    TransactionManager: manager,
    Transactions: transactionpgx.New(manager),
    Users: userpgx.New(manager),
    Identities: identitypgx.New(manager),
    Events: events,
    Access: access,
    Eligible: eligible,
}
// Supply deps to authorization.New with your existing policies and limits.
// Add authorization.WithSessionManager(sessions) to the tenant options and
// authorization.WithSessionIssuance() to each intent that should issue a session.
```

See the [compilable wiring example](../driver/transactionmanager/pgx/example_test.go).
The engine does not open or close connections, apply migrations at startup, or
select drivers automatically. Policies, access, eligibility and method drivers
remain host configuration. Challenges stay inside the authentication transaction.

## PostgreSQL contract

All mutations and `ForUpdate` reads require the context provided to
`manager.Do`. Ordinary reads can use a normal request context. Update/Delete
require a preceding locked Get of the same root in that scope. Nested scopes,
foreign manager contexts, escaped callback contexts and unsupported read options
are rejected. Never retain callback contexts or invoke callbacks concurrently
without awaiting all participating work before the callback returns.

Transactions persist a versioned JSON snapshot, including discriminated policy
nodes, private challenges, pending attempts, claims, receipts and completion
intent. Unpublished events are excluded. Existing immutable fields are checked
against the locked snapshot. Owner filters are exact, including an empty owner.
Users and identity subjects remain global, as defined by the domain contract.
PostgreSQL timestamps on users and identities have microsecond precision and
are returned in UTC.

SIGN_UP preserves the approved all-or-nothing transaction across root state,
new user, initial identity and events. A duplicate issuer/subject rolls back the
losing registration. Callbacks are never automatically replayed by the driver.
After a lost commit acknowledgement, retry the original engine operation so its
stored attempt and completion guards determine the result.

The event adapter uses [Vibox](https://github.com/not-for-prod/vibox) on the same
pgx transaction. Topic `authorization.transactions.v1` has 32 fixed partitions.
The partition key is the JSON encoding of `[tenantID, transactionID]`. The payload
is the existing `TransactionEvent` envelope, retaining domain event IDs, versions,
and correlation/causation metadata. Only W3C trace context is propagated.
Never change this topic's partition count or key encoding after publication.
Hosts configure Vibox relays and idempotent handlers for their destinations.
Publication means durable append, not completed delivery.

## Redis contract

`driver/session/redis` implements `gateway.SessionManager`, including credential
issuance, lookup, expiry and revocation. It does not implement the independent
logical `repository.Session` port, which is not used by engine dependencies.

Create is atomic by tenant and transaction key. Retries require the same
principal and return the original session and token within the originally stored
recovery deadline. Passing a later deadline does not extend retained recovery.
Hosts should reuse the original `RecoverUntil` on every retry.
Principal timestamps are compared as UTC instants. A repeated Create does not
extend session expiry or reactivate a revoked session. The recovery result may
therefore contain an already-revoked or expired credential, which Validate rejects.

The opaque token consists of a random session ID and an independent random secret.
Session lookup stores the token digest. The issuance record retains the original
credential because the engine requires exact credential recovery. Protect Redis
backups and credentials accordingly. No token or request payload is added to spans.
Keys include a hashed tenant hash tag so each Lua operation stays in one Redis
Cluster slot. Times and expiration checks use Redis TIME with millisecond precision.

Session and issuance records expire after the later of session expiry and recovery
deadline. Redis must retain acknowledged writes: configure persistence, replication
and `maxmemory-policy noeviction` for the required durability. The PostgreSQL/Redis
boundary is not a distributed transaction. Configure the injected client's
connection, read/write timeouts, TLS and credentials in the host; enable
`ContextTimeoutEnabled` when using go-redis clients so request deadlines also bound
socket I/O. The driver checks cancellation and propagates each request context.

## Migrations and local verification

```sh
make infra
make migrations-up
make xo
make drivers-test
```

The dedicated Compose project `authorization-drivers` exposes PostgreSQL on
`127.0.0.1:55449` and Redis on `127.0.0.1:56389`. The database is `authorization`.
Default credentials are for this local development setup only. `make infra-stop`
stops both services. Integration tests create and remove random PostgreSQL schemas
and random Redis tenant keys, without flushing Redis or resetting existing tables.

The ordered Goose migrations are under `tools/migrations` and also embedded as
`migrations.Files` for host migration tooling. Migration 00002 is copied from
Vibox commit `1cd812ffae79` with its license alongside it. Run migrations as a
singleton deployment step before constructing the publisher. Down migrations
remove stored data and event offsets, so use them only for an explicitly intended
rollback. Reapplying them does not restore that data.

The xo templates generate database rows and table/column constants only. Driver
methods own their parameterized SQL and scoped pgx calls. Regenerate with `make xo`
after schema changes; do not edit `generated/xo` by hand. Use a dedicated generation
database to avoid generating unrelated host tables.

## Redis authentication progress with PostgreSQL completion

Hosts can opt into `driver/transactionmanager/redis` and `driver/transaction/redis`
for expiring progress. Set `Dependencies.Completions` to
`driver/completion/pgx.New(pgManager)` and `Dependencies.CompletionManager` to the
same PostgreSQL manager used by Users, Identities and Events. Both optional fields
must be supplied together. Existing PostgreSQL-only configurations remain valid.
Apply migration 00004 before using this mode. The library retains its optional
PostgreSQL transaction table; hosts such as Atlas may remove that table after
stopping old writers and verifying their new configuration.

Redis scopes stage one bounded root and commit using an atomic comparison against
the originally read snapshot. A conflict returns Aborted; callbacks are never
automatically replayed. ForUpdate means a version-checked write in this adapter,
not a Redis lock. Use context-aware client timeouts and durable, noeviction Redis.
The absolute authentication deadline never extends on an update.

Method starts commit their claim before provider I/O, so concurrent starts cannot
dispatch twice. A failed or ambiguous start is not automatically replayed; begin a
new flow when its outcome cannot be recovered.

Final proof is frozen in Redis before PostgreSQL finalization. Completion receipt,
new signup user/identity and AuthenticationCompleted commit together under a
receipt-key lock. Session issuance happens afterward. Retry the original verify
or callback operation to recover a committed result without replaying proof.
Tenant, caller, attempt, challenge, operation kind and the original deadline are
checked. Recovery rechecks current user eligibility. Progress queries can use the
receipt after Redis progress disappears. Receipt retention must cover the original
deadline; no cleanup worker is added here. Receipts exclude challenge secrets and
tokens but contain private authentication metadata and need normal access controls.

Only signup and sign-in are implemented in staged mode. Existing unsupported
intents remain unsupported. The separate Redis short-lived-storage draft is not
the implementation contract for this option: there is no Redis outbox bridge or
background registration-recovery worker.
