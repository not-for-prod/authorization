# Acceptance criteria
1. User, identity, transaction and event writes commit together or roll back together on failure, cancellation, and panic.
2. Competing subject creation has one winner. Locked reads serialize mutation and unlock on rollback.
3. Owner/tenant mismatches return NotFound, including empty owner filters. Updates preserve immutable state and never upsert.
4. Persisted policy and nested transaction state round-trip without new events or aliasing.
5. Concurrent Redis issuance returns one credential. Different principals fail. Retry after response loss returns the original credential without extending expiry or undoing revocation.
## Success scenarios
Create and read each root, locked update/delete, durable publication, session create/validate/revoke and recovery across manager instances.
## Failure scenarios
Duplicate subject, missing record, unsupported option, foreign/escaped scope, missing lock, outbox failure, expired deadline, revoked/expired/foreign token, unavailable Redis.
## Regression checks
Existing engine tests and in-memory drivers remain valid.
## Required tests
Real PostgreSQL migration up/down, repository concurrency and atomicity integration tests, Redis Lua integration tests, versioned snapshot codec tests, full Go test suite and lint.
