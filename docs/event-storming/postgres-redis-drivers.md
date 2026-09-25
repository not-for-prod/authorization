# PostgreSQL and Redis drivers

Status: Approved by the owner in the implementation task on 2026-09-25.

Add pgx PostgreSQL adapters for every storage dependency in `Dependencies`:
Transactions, Users, Identities, TransactionManager, and Events. Events use the
required Vibox durable outbox in the same PostgreSQL transaction. Add Goose
migrations and host wiring documentation. Host configuration continues to own
policies, access, eligibility, and authentication methods.

Interpret “session store redis” as a Redis-backed tenant `SessionManager`, matching
the existing in-memory driver's Create, Validate, and Revoke surface. It owns
logical sessions, opaque credentials, and issuance recovery. A separate generic
`repository.Session` adapter is not required by engine dependencies.

## Happy path

```mermaid
sequenceDiagram
    actor Client
    participant App as Authentication application
    participant PG as PostgreSQL through pgx
    participant Redis as Redis session manager
    Client->>App: Start and Verify or Callback
    Note over App: Verify provider proof outside database locks
    App->>PG: Begin and lock authentication transaction
    App->>App: Accept proof and evaluate policy
    opt Successful SIGN_UP
        App->>PG: Create user and unique identity
    end
    App->>PG: Save root and append Vibox events atomically
    PG-->>App: Commit completion intent
    opt Session issuance enabled
        App->>Redis: Create using tenant and transaction key
        Redis-->>App: Stable session and opaque token
    end
    App->>PG: Lock root and commit completion with events
    App-->>Client: Authentication result
```

## Failure and recovery

```mermaid
sequenceDiagram
    actor Client
    participant App as Authentication application
    participant PG as PostgreSQL through pgx
    participant Redis as Redis session manager
    alt Identity conflict or storage or outbox failure
        App->>PG: Roll back all participating writes
        App-->>Client: Classified error
    else Commit acknowledgement lost
        Client->>App: Retry original operation
        App->>PG: Read and lock authoritative committed state
        App->>App: Resume using existing attempt and completion guards
    else Redis response lost or completion write fails
        Client->>App: Retry final attempt before recovery deadline
        App->>PG: Load committed completion intent
        App->>Redis: Repeat issuance key and principal
        Redis-->>App: Same session and token
        App->>PG: Commit completion and events under root lock
        App-->>Client: Recovered result
    end
    Note over App,Redis: Different principal for an existing key is rejected
```

## Session lifecycle

```mermaid
sequenceDiagram
    actor Host
    participant Redis as Redis session manager
    Host->>Redis: Concurrent Create with same tenant and transaction key
    Redis-->>Host: One atomic issuance and identical returned credentials
    Host->>Redis: Validate opaque token
    Redis-->>Host: Tenant session if active and unexpired
    Host->>Redis: Revoke session ID
    Redis-->>Host: Idempotent success
    Host->>Redis: Validate revoked or expired token
    Redis-->>Host: Unauthenticated
    Host->>Redis: Retry original issuance before recovery deadline
    Redis-->>Host: Original issuance without reactivating revoked session
```

## Aggregate boundaries

| Root | Purpose and owned state | Identity and concurrency |
| --- | --- | --- |
| Authentication transaction | Collect and finalize one authentication flow, owning bounded attempts, challenges, claims, evidence, receipts, policy/settings snapshots, and completion intent. | Tenant and transaction ID, exact owner-key filtering, root row locked before mutation until commit. |
| User | Represent one canonical subject, owning status and creation time. | Global user ID, unique creation, locked Get before Update. |
| Identity | Assign one stable provider subject to one user, owning kind, issuer, subject, and optional password hash. | Global identity ID and unique issuer/subject, locked Get before Delete. |
| Session | Represent one tenant login, owning principal, credential lookup, expiry, revocation, and bounded issuance recovery state. | Tenant and session ID, unique issuance by tenant and transaction key, atomic Redis operations. |

No new business aggregate or cross-aggregate event consumer is introduced.
The outbox is technical durable publication state, not a business root. Each
command appends a bounded set of existing transaction events without loading history.
Transactions reference user IDs, identities reference user IDs, and sessions copy
the final principal. Redis does not join a PostgreSQL transaction.

## Commands, events, and external effects

- Preserve existing Start, Verify, Callback, and completion commands and their
  transaction-owned events, including MethodStarted, EvidenceAccepted,
  UserRegistered, PolicySatisfied, and AuthenticationCompleted.
- Persist envelopes with their stable IDs, aggregate versions, and correlation
  metadata. Preserve existing snapshot redaction and exclude unpublished events
  from persisted transaction snapshots.
- No event drives authentication completion. Vibox relay/broker configuration is
  host-owned, with at-least-once delivery and idempotent consumers. This change
  supplies durable publication storage and wiring guidance, not a new broker.
- Redis issuance runs after committed completion intent and outside root locks.
  Retain issuance recovery data until both the session expiry and RecoverUntil
  have passed. An issuance retry must never extend expiry or reverse revocation.
- Redis persistence, replication, and eviction settings remain host deployment
  responsibilities. Recovery guarantees require retention of acknowledged data.

## Atomic sign-up exception for approval

Preserve the existing synchronous SIGN_UP boundary: lock the authentication
transaction, create User, create Identity, save transaction, append events, commit
once. No existing user or identity is mutated. Unique identity creation serializes
competing registrations, and any failure rolls back the entire write set.

The immediate invariant is that failed registration leaves no orphan account or
identity and no committed authentication binding. Eventual account creation would
introduce pending-account and compensation behavior beyond the current contract.
Lock order remains transaction first, then creation constraints. Constraint
conflicts reject the losing registration. Database deadlocks and transient failures
roll back and return classified errors without replaying callback side effects.
Owner approval of this document approves this narrow exception for the drivers.
Revisit it if registration introduces additional mutable roots or staged accounts.

## Scope and decisions

- Preserve existing domain contracts and in-memory drivers. Add optional drivers
  under the current `driver/<dependency>/<implementation>` layout.
- Include PostgreSQL migrations and integration verification for rollback, locks,
  immutable fields, snapshot round trips, uniqueness, and atomic event publication.
- Include Redis concurrency, tenant isolation, recovery, expiry, and revocation
  verification. No automatic deployment or destructive database reset.
- The owner approved this boundary map, atomic sign-up exception, and Redis
  SessionManager interpretation. Schema details and implementation documents are
  derived from that approval.
