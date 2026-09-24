# Acceptance criteria

## Acceptance criteria
No account before full policy satisfaction. Exactly one user owns a registered
identity. Failed account writes leave no orphan. Replaying completion does not
repeat proof or create an account/session again.

## Success scenarios
Direct identity proof across two requirements creates an active user. OAuth
callback creates a user. A session failure recovers the same registered UserID.

## Failure scenarios
Invalid proof, UserID-only proof, subject mismatch, eligibility denial, identity
conflict and event-publication failure cannot produce a new account.

## Regression checks
Existing sign-in, policy discovery, method attempts, constructor validation,
transport adapters and external-host tests pass after restoration.

## Required tests
sign_up_test.go covers partial proof, copy isolation, mismatched identity,
concurrent identity claims, rollback, eligibility, callbacks and issuance retry.
Run the application/domain race tests and repository-wide Go tests.
