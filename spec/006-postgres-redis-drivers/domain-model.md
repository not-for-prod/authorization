# Domain model
Derived from the owner-approved [visual model](../../docs/event-storming/postgres-redis-drivers.md), which owns the sequence diagrams, boundary table, event routes, and sign-up exception.

Transaction owns bounded method attempts, evidence, policy, claims and completion under one row lock. User owns status. Identity owns one unique issuer/subject link. Session owns credential lifecycle and issuance recovery in Redis. No new business commands or consumers.

Events retain the existing TransactionEvent envelope. The Vibox topic is authorization.transactions.v1 with 32 fixed partitions. Partition bytes are the JSON array [tenantID, transactionID], avoiding ambiguous concatenation. Domain event identity remains in the JSON payload. Event publication uses the same database transaction as root persistence.

Redis keys use a SHA-256 tenant hash tag and hashed issuance keys. An opaque token identifies a random session ID plus a random secret; lookup stores its hash. Atomic scripts bind the issuance key to the original principal and retain the result for max(session expiry, recovery deadline). Revoke changes only credential validity, never the issuance result.
