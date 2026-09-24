# First SIGN_UP implementation

The owner requested undoing the application removal and implementing one flow at
a time for correction. This supersedes the earlier approval-before-code sequence
for this first iteration; it is not a claim of final model approval.

Implement the synchronous [SIGN_UP model](../../docs/event-storming/sign-up.md).
Retain the existing application interface. A verified new identity becomes an
active user only when the complete policy is satisfied. Reuse normal optional,
idempotent session issuance. Existing-user flows are restored.
