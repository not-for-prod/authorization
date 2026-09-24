# Implementation phases
## Phase 1: Google driver
### task-1.1 Implement and test the Goth Google adapter
Goal: Satisfy AC1-3 and AC7 using the requested SDK.
Files:
- driver/method/goth/*
- go.mod, go.sum
Acceptance: typed configuration and proof inputs, secure correlation, bounded provider calls, sanitized errors and identity mapping.
Verification: GOCACHE=/private/tmp/authorization-go-cache go test ./driver/method/goth
## Phase 2: Verification transactions
### task-2.1 Fence callback and verification dispatch
Goal: Satisfy AC4-6 while preserving intent behavior.
Files:
- application/service/authentication-service/complete_callback.go
- application/service/authentication-service/verify_method.go
- driver/method/goth/engine_test.go
Acceptance: claim commit, provider I/O outside lock, atomic proof acceptance, definitive rejection and ambiguous recovery.
Verification: focused integration tests with race detector.
## Phase 3: Handoff
### task-3.1 Document host wiring and verify
Goal: Make configuration and callback association concrete.
Files:
- driver/method/goth/README.md
- driver/README.md
- docs/agents/init.md
Acceptance: executable wiring example and honest verification results.
Verification commands:
- GOCACHE=/private/tmp/authorization-go-cache go test ./...
- GOCACHE=/private/tmp/authorization-go-cache go test -race ./driver/method/goth
- make linter
## Checkpoints
Return for approval only for a new observable decision or scope expansion.
## Execution record
- task-1.1 complete: pinned Goth v1.82.0, added tenant-scoped Google configuration, typed inputs, private challenges, correlated callbacks, bounded context-aware SDK transport, sanitized errors and deterministic protocol tests.
- task-2.1 complete: Callback and Verify now commit exclusive claims before dispatch and reload locked state for rejection/acceptance. Tests cover both entrypoints, replay, caller/attempt checks, concurrent dispatch, late results, attempt budgets, ambiguous recovery and atomic event publication.
- task-3.1 complete: host setup/wiring/callback guide and compiling example added; driver catalog and design context updated.
- Verification passed: `go test -race ./driver/method/goth`; all driver and domain package results passed in `go test -race ./driver/... ./domain/... ./examples/...` (combined command failed only on concurrently edited `examples/main.go`).
- Verification passed: default-rule `golangci-lint run --no-config --tests=false ./driver/method/goth ./application/service/authentication-service` and `golangci-lint run --no-config ./driver/method/goth`, using `/private/tmp/authorization-golangci-lint` with writable Go/linter caches.
- `go mod tidy -diff` passed without changes. `go build ./...` passed before subsequent concurrent edits to the example host.
- `go test ./...` cannot complete: existing root/application/delivery tests import missing `internal/testutil/authfixture`. The sandbox also blocked unrelated Telegram local-server tests; those passed when rerun with socket access.
- `make linter` cannot run because `.golangci.yaml` is missing. Default-rule checks above do not substitute for that unavailable repository lint contract.
- No live Google credentials were used. No protobuf, generated code or schema changes were required. Concurrent Telegram/login-password files and dependencies were preserved. The spec was moved to 005 to avoid the concurrent 004 feature numbering.
