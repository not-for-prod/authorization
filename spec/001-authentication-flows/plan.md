# Implementation plan
Status: Complete. Implemented the approved existing-user model.

## Implementation phases
Contracts/domain; application; delivery; documentation and final verification.

## Tasks

## Phase 1: Domain and source contracts
### task-1.1 Transaction lifecycle and session idempotency
Goal: Protect the approved root invariants and recover session issuance.
Files:
- domain/entity/{transaction,challenge,policy}.go
- domain/gateway/session_manager.go
- domain/repository/{transaction,challenge}.go
- generated/mocks
Acceptance:
- Bounded claims/challenges, immutable snapshots, subject binding, expiry,
  rejection and completion guards; source contracts and mocks agree.
Verification:
- go test ./domain/...
- go generate ./domain/gateway ./domain/repository
Status: Complete. Domain tests pass; mocks regenerated with moq v0.7.1 built on Go 1.26.5.

## Phase 2: Application
### task-2.1 Execute all authentication operations
Goal: Coordinate host access, methods, repositories and recoverable completion.
Files:
- application/service/authentication-service
Acceptance:
- Direct/MFA/callback flows, isolation, locking, rollback, stale claims and
  completion retries meet acceptance_criteria.md.
Verification:
- go test -race ./application/...
Status: Complete. Application race tests cover success, rollback, claims and completion recovery.

## Phase 3: Delivery
### task-3.1 Implement Clay handlers and HTTP integration
Goal: Expose all five existing methods through validated, safe mapping.
Files:
- delivery/api/authentication-service
- internal/testutil/authfixture
Acceptance:
- Real HTTP requests exercise the application; decoder uses stored callback
  selection; responses omit private state and errors omit secrets.
Verification:
- go test -race ./delivery/...
Status: Complete. Clay HTTP integration tests pass under the race detector.

## Phase 4: Handoff
### task-4.1 Document and verify the runnable implementation
Goal: Make embedding and test commands reproducible.
Files:
- docs/authentication-*.md
- docs/agents/init.md
Acceptance:
- Documentation reflects actual contracts and explicit limitations.
Verification:
- go test -race ./...
- go vet ./...
- make linter (report missing config if still absent)
Status: Complete. Docs updated; full race tests, go vet, and default-config lint pass. Project-config lint is blocked by missing .golangci.yaml.

## Checkpoints
Only new observable decisions, expanded authority or destructive actions require
approval; do not pause for routine tasks implementing the approved model.

## Verification commands
Use cached Go 1.26.5 and a writable temporary GOCACHE. Run focused checks above,
then broad checks once, rerunning only after relevant fixes.
