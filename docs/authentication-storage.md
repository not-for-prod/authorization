# Authentication storage contracts

Storage interfaces live under [domain/repository](../domain/repository).
The host supplies implementations. These are ports, not database implementations,
and no migration or infrastructure dependency is introduced.

| Interface | Operations | Purpose |
| --- | --- | --- |
| `User` | Create, Get, Update | Store the canonical internal subject and its state. |
| `Identity` | Create, Get, GetBySubject, Delete | Resolve an issuer/subject pair to one canonical user. |
| `Transaction` | Create, Get, Update | Persist an authentication transaction and its evidence. |
| `Challenge` | Get | Optional read projection of a transaction-owned method attempt. |
| `Session` | Create, Get, Delete | Store logical sessions for a stateful session adapter. |
| `Policy` | Get | Retrieve the policy for a tenant and intent, including from host configuration. |
| `EventPublisher` | Publish | Append aggregate events atomically with their state. |
| `TransactionManager` | Do | Scope an adapter's storage transaction and locks. |

`gateway.SessionManager` separately owns credential issuance, validation, and
revocation. An opaque-token implementation owns the mapping from its credential
to a logical session; the session repository does not store raw tokens. A JWT
implementation can use a different persistence strategy entirely.

## Adapter contract

Repositories persist state already decided by the domain/application layer.
They do not evaluate policies, bind a different user, consume proofs, or decide
whether an authentication flow may complete.

Create must not overwrite existing records, and Update must not upsert a missing
record. Missing records preserve `repository.ErrNotFound`; uniqueness conflicts
preserve `repository.ErrAlreadyExists`, including when an adapter adds its error
classification. Delete also returns `ErrNotFound` when its target is absent.
Identity creation enforces unique IDs and unique issuer/subject pairs without
merging users or replacing existing identity ownership.

Reads return independent snapshots, including nested slices and pointers.
Adapters persist their own copy rather than retaining mutable caller references.
Tenant-scoped reads must check all supplied keys. `Transaction.Get` requires
`tenantID`, `id`, and `ownerKey` before optional locking options. All three must
match; an owner mismatch returns `ErrNotFound`, just like a missing transaction.
An empty owner key is an exact filter, never a wildcard. For challenges, tenant and
transaction ownership must both match. Users and identities use the global IDs
and issuer/subject uniqueness in the current design context.

An existing mutable root is read with `repository.ForUpdate()` inside
`TransactionManager.Do`, changed through domain behavior, and then persisted
using the callback context. The adapter holds that root lock until commit or
rollback and rejects lock requests outside its transaction scope. Unsupported
read options must fail instead of silently losing concurrency protection.
These obligations must be verified by each concrete adapter's integration tests.

## Flow implementation boundary

The authentication transaction owns its challenges, policy snapshot, settings,
caller association, pending attempt, bounded receipts, version, claim, evidence,
final principal, and completion operation.
Its repository must persist them atomically under the transaction root lock.
Challenge consumption and evidence acceptance cannot be committed separately.
The `Challenge` projection has no write methods and is not used by orchestration.
User and Identity repositories are read-only in the implemented authentication
flows. Session persistence belongs to the idempotent session gateway.

`Transaction.Copy()` returns an independent snapshot of root-owned state, excluding
unpublished events. The application passes `Transaction.Events()` to `EventPublisher`
after persistence inside the same `Do` callback. Publisher failure must roll back
both state and events. Rehydration and successful retries emit nothing. The
adapter must preserve immutable identity, tenant, intent, caller, creation/expiry,
policy, and settings across updates. Raw session tokens and method inputs are
never persisted by this repository.

`TransactionManager` represents a storage transaction, not an authentication
transaction. A manager does not create a distributed transaction over separately
configured stores. Remote provider calls must occur outside locked transactions.
Credential storage, enrollment, cleanup scans, and identity listing can acquire
additional contracts when their concrete use cases are implemented.

## In-memory driver

Construct `driver/transactionmanager/inmem.New()` first, then pass that manager
to `driver/transaction/inmem.New`, `driver/user/inmem.New`, and
`driver/identity/inmem.New`, plus `driver/event/inmem.New`. Inject each chosen component into its corresponding
`authorization.Dependencies` field. Mutations and locked reads require the callback
context from `manager.Do`. Errors, panics and cancellation roll back the callback's
writes; nested, foreign and expired transaction contexts are rejected.

Transactions serialize across the store. Reads return independent snapshots,
updates require a locked read, and identities enforce unique IDs and
`(issuer, subject)` pairs. Challenges persist inside the transaction aggregate.
The event driver stores an atomic in-memory outbox; `List(ctx)` returns independent
event snapshots in commit order. It has no relay or consumer and loses events on
restart. Persistent hosts must provide a durable publisher sharing their root
transaction (see [domain event integration](domain-events.md)).
The driver is process-local, retains records until discarded, and is intended
for development and tests, not durable or distributed storage.

Optional opaque sessions use `authorization/driver/session/inmem`, configured
separately per tenant. Access, eligibility and authentication methods remain
host responsibilities. No session implementation is selected by default.
