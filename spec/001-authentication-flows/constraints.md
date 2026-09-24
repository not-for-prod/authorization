# Constraints
Status: Derived from approved model; prompt/02 applied.

## Layer ownership
Domain owns transitions/ports; application coordinates; delivery validates/maps.
Host constructs adapters. There is no cmd/Fx tree: expose constructors and a Clay
descriptor, exercised through HTTP tests, without creating a new server surface.

## API and protobuf constraints
Keep service.proto, authorization.v1, tenant routes and response fields unchanged.

## Domain constraints
Authentication transaction owns bounded challenges, claims, policy snapshot and
completion intent. User/Identity remain read-only. Keep related types together.
No event publication/outbox is introduced: this is the explicit initial-slice
exception in the owner-approved model. Revisit when a consumer or cross-root
write is introduced; do not silently invent a broker contract.

## Application service constraints
Use one public method per file. Claim before dispatch, call outside locks, reject
stale results, and retire abandoned transactions on mutating retries. Get is pure.
Require host access and eligibility callbacks. Copy inputs passed to drivers.

## Repository constraints
Root Get(ForUpdate) inside TransactionManager.Do precedes every update. Persist
challenges/evidence/completion atomically. Use callback context and independent
snapshots. No independent challenge writes.

## Gateway constraints
Methods return verified facts. Unknown outcomes retain claims, definitive proof
rejections permit bounded retries. Session Create is idempotent by tenant/key,
returns the same credential until recovery expiry, and rejects changed principal.

## Migration constraints
No concrete database or migration in this test slice.

## Error handling constraints
Classify origin errors with proterror; preserve domain sentinels. Map classified
errors to safe transport responses without exposing provider input or credentials.

## Tracing and observability constraints
Use prospan at application/delivery entrypoints. Never attach input or tokens.
Host adapters must return sanitized errors.

## Generation constraints
Regenerate affected moq outputs using Go 1.26-compatible tools; never hand-edit.
The owner supplied prompt/ during execution; all derived artifacts are now
reconciled with the provided prompts, without changing the approved scope.
