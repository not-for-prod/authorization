Status: Completed on 2026-09-23.

# Implementation phases
1. Domain attempts and events.
2. Atomic in-memory publication.
3. Application and API contracts.
4. Flow tests and documentation.
# Tasks
## Phase 1: Domain
### [x] task-1.1 Pending attempts and transitions
Goal: enforce the approved lifecycle and emit immutable events.
Files: domain/entity.
Acceptance: starts, matching verification, replay and policy transitions.
Verification: go test ./domain/...
## Phase 2: Storage
### [x] task-2.1 Publisher and atomic outbox
Goal: commit events with state under the shared manager.
Files: domain/repository, driver/event/inmem, driver/internal/memory.
Acceptance: rollback, stable event identity and independent snapshots.
Verification: go test ./driver/...
## Phase 3: Application and transport
### [x] task-3.1 Wire event publication and attempt contracts
Goal: explicit commands and synchronous completion.
Files: application, delivery, api, examples, public constructor.
Acceptance: required attempt IDs, pending progress, generated bindings.
Verification: make pb and focused flow tests.
## Phase 4: Verification
### [x] task-4.1 Migrate and extend tests/docs
Goal: complete public and embedded examples.
Files: tests and docs.
Acceptance: full race suite and lint.
Verification: go test -race ./...
# Checkpoints
Only new scope or architecture decisions.
# Verification commands
make pb, go test -race ./..., compatible golangci-lint.
