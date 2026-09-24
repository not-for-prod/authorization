# Domain model

The authoritative approved model is [authentication-method-attempts](../../docs/event-storming/authentication-method-attempts.md).
One authentication transaction is the only modified business root. Its attempt, challenge and receipts are bounded owned state. User and identity roots are read-only, and session issuance uses the host idempotency contract outside locks.
