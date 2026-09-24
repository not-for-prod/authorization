# Account lifecycle intents

Status: historical broad proposal. The owner selected implementation-first review,
one flow at a time. Current SIGN_UP behavior is documented in [sign-up.md](sign-up.md).
Do not infer UPDATE or DELETE implementation from this earlier proposal.

The owner selected four closed intents and separate application business flows:
SIGN_UP, SIGN_IN, UPDATE (including reset), and DELETE. All reuse method
attempts, evidence collection, and policy evaluation. This replaces the earlier
proposal to automatically create missing users during authentication.

## Shared proof collection and intent dispatch

```mermaid
sequenceDiagram
    actor Client
    participant App as Application
    participant Method as Configured method
    participant Tx as Authentication transaction
    participant Flow as Selected lifecycle flow
    Client->>App: Start intent with its target and requested change
    App->>Tx: Freeze caller, tenant, intent, policy and operation
    loop Until policy is satisfied
        Client->>App: Start and verify method attempt
        App->>Method: Verify proof outside storage locks
        Method-->>App: Verified subject and evidence
        App->>Tx: Accept appropriate proof and evaluate policy
    end
    Tx-->>App: Policy satisfied for this exact operation
    App->>Flow: Execute the selected intent
    Flow-->>Client: Completed result or pending completion
```

The requested change cannot be replaced after proof collection begins. Proof of
an existing account and proof of a proposed new identity are distinct roles.
For example, possession of a new email does not authorize changing someone
else's account. Raw passwords and tokens never enter evidence, public progress,
or event snapshots. Credential preparation produces a protected enrollment input,
not authentication evidence.

## SIGN_UP and SIGN_IN

```mermaid
sequenceDiagram
    actor Client
    participant App as Application
    participant Tx as Authentication transaction
    participant User as User aggregate
    participant Sessions as Session manager
    Client->>App: Complete required evidence
    alt SIGN_UP
        App->>Tx: Retain verified unregistered identities without UserID
        Tx-->>App: LifecycleActionAuthorized after full policy satisfaction
        App->>User: Create user with initial identities and prepared credentials
        User-->>App: UserCreated or identity conflict
        App->>Tx: Record creation result and bind new UserID
    else SIGN_IN
        App->>User: Resolve existing user and check eligibility
        App->>Tx: Bind same user across accepted proofs
        Tx-->>App: AuthenticationSatisfied after full policy satisfaction
    end
    opt Session issuance configured and successful eligible completion
        App->>Sessions: Issue with stable transaction key
        Sessions-->>App: Same session on retry
    end
    App-->>Client: Completed account result
```

SIGN_UP is new-account-only. An identity already owned by another account is a
conflict, not an implicit sign-in or merge. A verified arbitrary UserID cannot
create a user. SIGN_IN rejects missing users and never enrolls credentials.
Password enrollment is distinct from comparing an existing password hash.

## UPDATE and DELETE

```mermaid
sequenceDiagram
    actor Client
    participant App as Application
    participant Tx as Authentication transaction
    participant User as User aggregate
    participant Sessions as Session manager
    Client->>App: Start UPDATE or DELETE with exact target and operation
    App->>Tx: Collect required existing-user or recovery evidence
    opt UPDATE introduces a new identity
        App->>Tx: Collect separate proof of the new identity
    end
    Tx-->>App: LifecycleActionAuthorized
    alt UPDATE
        App->>User: Apply the authorized identity or credential change
        User-->>App: UserUpdated
    else DELETE
        App->>User: Apply approved account deletion behavior
        User-->>App: UserDeleted
    end
    App->>Sessions: Invalidate sessions required by the approved operation policy
    Sessions-->>App: Invalidation acknowledged
    App->>Tx: Record operation outcome
    App-->>Client: Completed without issuing a new session
```

Recovery may begin without a signed-in caller but must prove the existing target
user through the configured recovery policy. A new credential is never accepted
as proof of the old account. Profile-field support and deletion semantics remain
open below. No success response precedes required session invalidation.

## Failure, concurrency, and recovery

```mermaid
sequenceDiagram
    actor Client
    participant App as Application
    participant Tx as Authentication transaction
    participant User as User aggregate
    participant Relay as Durable retry delivery
    alt Invalid proof or policy incomplete
        App-->>Client: Reject proof or return remaining requirements
        Note over Tx,User: No user mutation is authorized
    else Policy satisfied
        App->>Tx: Persist LifecycleActionAuthorized atomically with outbox
        Tx-->>Relay: Deliver authorized operation
        Relay->>User: Apply command using stable operation key
        alt Concurrent SIGN_UP already owns the identity
            User-->>Relay: Persist deterministic conflict result
        else Command succeeds
            User-->>Relay: Persist mutation and result event atomically
        end
        Relay->>Tx: Record operation result idempotently
    end
    opt Response lost or delivery interrupted
        Client->>App: Resume original transaction
        App->>Tx: Read recorded result or pending state
        Relay->>User: Replay same command if necessary
        User-->>Relay: Same recorded outcome without a second mutation
        App-->>Client: Original result or pending progress
    end
```

Proof rejection is not account absence. Storage failures are not registration
signals. Duplicate or out-of-order events cannot apply an operation twice or
change its payload. Poison events retain a failed/pending outcome for inspection;
they never authorize a replacement operation. An expired or canceled transaction
cannot authorize a new action. Once an action is durably authorized and dispatched,
recovery reconciles its committed outcome rather than pretending cancellation
rolled it back. Issuance remains subject to its existing recovery deadline.

## Proposed aggregate roots

| Root | Purpose | Identity and lock | Owned state and immediate invariants |
| --- | --- | --- | --- |
| Authentication transaction | Authorize one exact lifecycle operation through collected proof. | TransactionID, existing root lock. | Immutable target/change, policy snapshot, bounded attempts, proofs and outcome. Each proof has its intended role and one accepted operation cannot change. |
| User | Protect one user's authentication account. | UserID, user root lock for changes. SIGN_UP also enforces unique issuer/subject. | Bounded identities and credentials, lifecycle state, operation result receipts. Identity ownership and credential changes commit through one user write repository. Proposed configurable bound: 16 identities and 16 credentials per user. |
| Session | Control the validity of an issued authentication credential. | Existing host session manager and its stable issuance key. | Session/token state remains outside User and Transaction. Invalidation is acknowledged before UPDATE/DELETE reports completion where required. |

Identity and credential writes go through User commands and its repository,
including writes currently exposed by the standalone identity repository.
Lookup ports can remain available for method verification. User operation receipts
are technical idempotency records accessed by key, not an unbounded collection
loaded with the root. Retention must cover replay of every corresponding event.
No root locks span provider calls, session calls, or mutations of another root.

## Commands, facts, and cross-root routes

| Source fact | Next command | Idempotency and ordering |
| --- | --- | --- |
| Transaction `LifecycleActionAuthorized` | User `Create`, `Update`, or `Delete` | TransactionID plus immutable operation digest. Recheck ownership and eligibility at mutation time. |
| User `UserCreated`, `UserUpdated`, `UserDeleted`, or recorded rejection | Transaction `RecordLifecycleOutcome` | Same operation key and UserID. Cannot complete a different action. |
| Recorded successful result requiring session invalidation | Session manager invalidation command | Stable operation key. Retry or reconcile ambiguous outcomes before completion. |
| Recorded SIGN_UP result or `AuthenticationSatisfied` | Optional session issuance | Existing transaction-key issuance contract. |
| Final action result and required external acknowledgements | Transaction `Complete` | Same durable operation result. Duplicate completion is a no-op. |

Local commands may be attempted synchronously for an immediate response, but the
durable event route recovers interrupted cross-root work. Pending completion is
visible until the account outcome and required external effects are recorded.
Production publication uses the project's Vibox boundary through domain ports;
the demo needs an explicit in-memory relay and remains non-durable. This introduces
new user events and consumers beyond the current transaction-only event contract.
There is no multi-root transaction exception in this proposal.

## Decisions requiring owner review

1. SIGN_UP means new-account-only, with conflicts rejected. SIGN_IN never creates.
2. UPDATE binds evidence to a concrete change and separates target-account proof
   from proof of a newly linked identity. Proposed initial scope: identities and
   credentials, excluding arbitrary host profile fields.
3. DELETE needs a choice: reversible disable/tombstone or permanent removal.
   Identity reuse, retained audit/idempotency facts, tenant scope, and session
   invalidation must follow that choice. Users and identities are currently global,
   so deleting a global user must require explicit host authorization beyond
   ordinary access to one tenant.
4. Proposed session behavior: SIGN_UP/SIGN_IN preserve configured optional
   issuance. UPDATE/DELETE issue no new session. Reset and deletion invalidate
   existing sessions before reporting success. Current SessionManager lacks
   user-wide invalidation and must gain that capability.
5. Approve the User ownership boundary and bounded enrollment collections above,
   plus durable cross-root completion with visible pending progress.
6. Initial user status and enrollment eligibility remain host-configured facts;
   the service owns the creation command and writes. Concrete password hashing
   and provider enrollment adapters require explicit contracts and verification.

## Existing-system disposition

Preserve method attempts, evidence scoping, caller/tenant guards, pure policy
requirements, rollback, and idempotent session issuance. Replace unconditional
existing-user resolution and generic finalization with intent-specific behavior.
This proposal supersedes `authentication-user-creation.md` if approved. No new
feature spec or production code has been changed before this review.
