# Acceptance criteria
1. A provisioned identity authenticates only with its correct password, preserving exact issuer/login and canonical UserID.
2. Missing login, hashless identity and wrong password have the same definitive error and no evidence. Dummy success cannot authenticate.
3. Malformed input/selection and conflicting bound user cannot authenticate.
4. Storage/hash failures never expose raw errors, passwords or hashes, and never masquerade as valid proof.
5. Identity hash survives commit and reads, does not leak through rollback, and changing a returned value does not change storage.
6. Real engine Start/Verify completes a sign-in and existing policy/session behavior is preserved.
## Success scenarios
Sign-in from an unbound transaction and verification matching an already-bound user.
## Failure scenarios
Missing identity/hash, wrong password, invalid type, nil input, oversized fields/hash, corrupt hash, repository error, canceled context, wrong intent/provider/method/challenge and mismatched subject.
## Regression checks
Other method drivers and hashless identity flows still compile and pass. No credentials are added to engine events or results.
## Required tests
Deterministic callback and repository tests, in-memory credential round-trip/rollback test, engine integration, full Go suite and lint attempt.
