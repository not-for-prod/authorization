# Authentication domain events

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
A production adapter is not provided until a concrete persistent host is introduced.

Events describe committed user actions; they do not trigger authentication
finalization. Verify and Callback synchronously issue an optional session outside
the root lock, then commit completion. Retrying the final attempt recovers issuance
using the existing stable transaction key. No-op retries emit no additional events.
