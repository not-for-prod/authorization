# Acceptance criteria
- AC1: Google redirect uses the configured client, fixed callback URL, identity scope and unpredictable state.
- AC2: Matching callback returns only selection-scoped evidence and stable issuer/subject identity.
- AC3: Invalid or cross-tenant/challenge proof never reaches the provider.
- AC4: Callback and Verify perform network I/O outside storage transactions and concurrent requests exchange at most once.
- AC5: Definitive rejection persists rejection and permits bounded retry; uncertain errors remain fenced until abandonment.
- AC6: Accepted proof consumes private state and duplicate callbacks do not re-exchange.
- AC7: Errors and stored/event data contain no provider credentials, tokens or authorization codes.
## Success scenarios
Sign up with a new Google subject, then sign in with that identity. Partial policy progress remains supported. Independent tenant clients remain isolated.
## Failure scenarios
Denied consent, wrong state, invalid input, empty subject, provider invalid_grant, provider outage, timeout/cancellation, failed persistence and expired claims.
## Regression checks
Existing subject resolution, intent finalization, policy selection, caller ownership and attempt ID checks remain enforced.
## Required tests
Deterministic Google HTTP transport tests, real in-memory engine integration tests for both verification entrypoints, concurrency/claim recovery and event rollback tests. No live credentials needed.
