# Authentication with user creation when missing

Status: proposed. The owner selected one flow that creates the user if missing.
This review extends the approved existing-user-only authentication model.

## Happy path

```mermaid
sequenceDiagram
    actor Client
    participant App as Authentication service
    participant Driver as Configured method
    participant Tx as Authentication transaction
    participant Users as User and identity storage
    participant Sessions as Session manager
    Client->>App: Authenticate using the same intent
    App->>Driver: Verify proof outside storage locks
    Driver-->>App: Verified issuer and subject
    App->>Users: Resolve verified identity
    alt Identity already exists
        Users-->>App: Existing user
        App->>App: Check user eligibility
    else Identity is missing
        App->>Tx: Retain verified identity until policy is satisfied
        Note over App,Tx: No user or identity is created from an incomplete policy
    end
    App->>App: Confirm policy is satisfied
    App->>Users: Resolve again or create user with its verified identity
    Note over App,Users: Proposed atomic boundary includes new user, identity, and transaction binding
    App->>Tx: Persist final principal and completion intent
    App->>Sessions: Issue using the existing stable transaction key
    Sessions-->>Client: Principal and session
```

## Rejection and concurrent first authentication

```mermaid
sequenceDiagram
    actor Client
    participant App as Authentication service
    participant Driver as Configured method
    participant Users as User and identity storage
    Client->>App: Authenticate
    App->>Driver: Verify proof
    alt Invalid proof or incomplete policy
        App-->>Client: Reject or request remaining proof
        Note over App,Users: No account writes
    else Another transaction created the identity first
        App->>Users: Resolve unique issuer and subject under atomic creation boundary
        Users-->>App: The existing canonical user
        App->>App: Check eligibility of the resolved user
        App-->>Client: Complete authentication for that user
    else Existing user is blocked or ineligible
        App-->>Client: Reject without creating a replacement user
    end
```

## Failure and recovery

```mermaid
sequenceDiagram
    actor Client
    participant App as Authentication service
    participant Store as Shared storage transaction
    participant Sessions as Session manager
    alt A user, identity, or binding write fails
        App->>Store: Roll back the complete registration write set
        App-->>Client: Retryable failure
    else Session call or final response is interrupted
        App->>Store: User, identity, and completion intent already committed
        Client->>App: Retry original completion
        App->>Store: Read the same bound canonical user
        App->>Sessions: Issue with the same stable key and principal
        Sessions-->>Client: Same credential
    end
```

## Boundaries

| Root | Purpose | Creation/write boundary |
| --- | --- | --- |
| Authentication transaction | Protect one proof-collection and completion attempt. | Existing root lock, bounded verified identity data, immutable policy and caller binding. |
| User | Represent one canonical subject. | Create only after policy satisfaction; never upsert over a blocked or existing user's data. |
| Identity | Give a verified issuer/subject pair exactly one owner. | Unique issuer/subject and identity ID; never reassign or merge by email. |
| Session | Represent the issued credential. | Existing idempotent host session manager, outside storage locks. |

Commands and state-change facts: accept proof → `EvidenceAccepted`; create a
missing canonical subject → `UserCreated`; create the initial verified identity
→ `IdentityCreated`; bind the final subject → `AuthenticationSatisfied`;
complete issuance → `AuthenticationCompleted`. Resolving an existing user is a
read and emits no creation fact.

## Decisions for approval

1. One explicitly configured intent resolves or creates a user. Intent names
   themselves remain arbitrary and do not select behavior. No separate sign-up
   endpoint and no implicit user creation in existing-only configurations.
2. Only a driver-verified issuer/subject may create a user. An arbitrary returned
   UserID cannot create one. No account writes occur before policy satisfaction.
3. Concurrent creation and repeated completion resolve to the same user. Existing
   identity matches authenticate that user; blocked/ineligible users stay rejected.
   The host owns initial user attributes and eligibility; the engine does not
   invent a universal initial status or tenant-membership rule.
4. The demo uses a mock primary identity proof for this flow. Its hardcoded TOTP
   factor is removed from this demo policy because newly created users have no
   enrollment. Production MFA enrollment and password storage are outside this
   slice. Existing MFA engine behavior remains available to other policies.
5. **Proposed narrow architecture exception:** creation of a User, its initial
   Identity, and the authentication transaction's binding commit together in the
   shared in-memory transaction manager. Lock the authentication transaction first;
   serialize creation by unique issuer/subject, and re-resolve a winning existing
   identity. No provider or session calls occur while these locks are held.
   This prevents partially committed registration or a completion bound to a
   losing concurrent user ID. Any write failure rolls back the complete write set.
6. This example extension proposes no durable event broker/outbox, extending the
   initial model's exception for this test slice. Facts above describe the atomic
   transitions, not published messages. Owner approval is required for this
   multi-root/no-outbox exception. Before production persistence, review it against
   a host-owned atomic registration port or durable event-driven orchestration.

The existing approved model does not cover decisions 2–6. No application/domain
registration changes have been made pending review. The independent in-memory
user-mock correction can be used now for seeded existing-user tests.
