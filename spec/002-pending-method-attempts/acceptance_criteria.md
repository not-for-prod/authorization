# Acceptance criteria
No Verify without Start. Wrong or stale attempt IDs never call drivers. Concurrent Start dispatches once. Evidence consumes its attempt and evaluates policy atomically. Completion returns a session synchronously.
# Success scenarios
Start password, Verify password, Start MFA, Verify MFA, recover final response.
# Failure scenarios
Unstarted verification, mismatched selection, expired claims, wrong password, missing subject, outbox failure.
# Regression checks
Authorization, token-free reads, stable session issuance, bounded retries and callback correlation.
# Required tests
Domain transitions/events, event snapshot independence, storage rollback with outbox, API attempt-ID validation, HTTP flow, concurrency, full race suite.
