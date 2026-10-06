# Authentication domain events

Events are published only when an intent flow successfully completes: sign-up,
sign-in, identity changes, or account deletion. Intermediate state changes,
including transaction creation, method attempts, accepted evidence, policy
satisfaction, rejection and abandonment, do not publish events.

Currently sign-up and sign-in emit one `AuthenticationCompleted` event per
transaction. `After.Intent` identifies the completed intent and `After.UserID`
identifies the user. Identity-update and account-deletion flows are not implemented;
their events belong at successful business completion, not proof acceptance.
`UserRegistered` is no longer emitted separately during sign-up. Consumers of
intermediate events must move to the completion event; this is a deliberate
reduction of the published contract.

Aggregate versions still advance for internal transitions, so published event
versions need not be consecutive. No-op completion retries emit nothing.

Authentication transactions emit `entity.TransactionEvent` envelopes containing
stable IDs, ordered aggregate versions, occurrence/correlation/causation metadata,
and independent before/after snapshots. Snapshots exclude caller keys, method
inputs, private challenge payloads, redirects and session credentials.

Application commands persist the root, then call `repository.EventPublisher.Publish`
inside the same `TransactionManager.Do` callback. The publisher must participate in
that storage transaction. Errors roll back both writes. `Transaction.Copy` excludes
unpublished events; reconstructing persisted state does not create new events.

`driver/event/inmem` is the approved development implementation. Construct it with
the same manager as the transaction repository. Its outbox is atomic, process-local
and non-durable. `List` inspects copies of committed events. There is no background
relay, inbox or consumer in this implementation.

A persistent host must implement the port using Vibox with its storage transaction.
Keep record encoding, topic selection, partitioning and relay code in the host's
infrastructure adapter. Partition by tenant and transaction ID, preserve event ID
and aggregate version, and use at-least-once delivery with idempotent consumers.
`driver/event/pgx` provides this adapter using the same pgx transaction manager as the storage drivers. See [PostgreSQL and Redis drivers](postgres-redis-drivers.md) for migrations, topic contracts, and host wiring.

Events describe committed user actions; they do not trigger authentication
finalization. Verify and Callback synchronously issue an optional session outside
the root lock, then commit completion. Retrying the final attempt recovers issuance
using the existing stable transaction key. No-op retries emit no additional events.
