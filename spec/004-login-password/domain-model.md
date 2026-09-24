# Aggregate boundaries
Derived from the approved login/password model.
## Scenario
A host asks to verify an existing local identity. Success proves its canonical user at credential-read time. Account eligibility and proof acceptance remain engine-owned.
## Candidate Aggregate Roots
| Root | Purpose | Key | Owned state | Excluded state | Invariant | Lock | Bounded write set | Repository |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Identity | Link one issuer/subject to a user | ID and unique issuer/subject | One optional encoded hash | User, sessions, attempts | Hash proves only linked UserID | Existing Get ForUpdate before Delete | One identity | Identity |
| Authentication transaction | Establish a subject under a policy | Tenant and transaction ID | Attempts, evidence | Credentials | Proof matches attempt and bound subject | Existing Transaction.Get ForUpdate | Existing bounded root | Transaction |
## Relationship Decisions
Identity owns the single hash immediately; it has no independent lifecycle in this feature. Identity references UserID. Driver returns a proof referencing that user without mutating either root. Read-time proof semantics match the existing engine, with no new event route or revocation guarantee.
## Command Transactions
Host identity Create writes one identity with uniqueness enforcement. Existing StartMethod/VerifyMethod own engine state and events. Driver performs reads only using the supplied context. No new multi-root command.
## Vocabulary Check
Identity, password hash, login and verification use existing domain language. No new business root.
## Boundary Challenges
Keep one hash with Identity because it cannot be provisioned or addressed independently in this feature. Do not create a separate credential aggregate without a rotation/enrollment lifecycle.
## Exceptions
No new exception. Current engine locking/finalization is unchanged.
## Approval
Owner approved the visual model on 2026-09-24. No blocking questions.
