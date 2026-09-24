# Acceptance criteria
Status: Derived from approved model.

## Acceptance criteria
1. HTTP creation/get/start/verify/callback invoke real application code.
2. Password followed by MFA creates exactly one logical session, only after the
   policy is satisfied and both proofs resolve to the same eligible user.
3. Challenges remain private, are tenant/transaction/selection bound, and consume
   atomically with evidence. Callbacks use stored selection.
4. Owner mismatch, wrong tenant, expiry, unsupported intent/method, invalid proof,
   conflicting user, and concurrent attempts cannot issue a session.
5. A changed policy repository cannot change an existing transaction's policy.
6. A session error or lost commit/response can be retried with the original
   completion operation; the same issuance key and principal are reused.
7. Expired claims reject late results and retire on mutating retry. Get is read-only.
8. Tests use fakes/mocks, demonstrate rollback and race safety, and require no
   external database or provider.

## Success scenarios
Direct password then TOTP; start challenge then callback; principal-only step-up;
recover a session after its creation response or transaction commit is lost.

## Failure scenarios
Invalid/missing fields, unauthorized caller or tenant, unknown subject, identity
conflict, unavailable method, expired challenge/transaction/claim, stale result,
attempt limit, failed save, driver timeout, and malformed driver result.

## Regression checks
Existing policy evaluator tests continue passing. No session token, challenge
state, evidence or user binding appears in public progress. Get causes no writes.

## Required tests
Domain transitions/copy isolation; application rollback/concurrency/recovery;
HTTP integration across all five generated routes; safe error mapping.
