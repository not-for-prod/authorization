# Implementation phases
Driver, security verification, host documentation.
# Tasks
## Phase 1: Driver
### task-1.1 Implement the adapter
Goal: Enforce approved protocol behind existing interfaces.
Files: driver/method/telegram/*.go, go.mod, go.sum.
Acceptance: Typed inputs/config, private bound challenge, PKCE exchange, RS256/JWKS validation and sanitized errors.
Verification: go test ./driver/method/telegram
## Phase 2: Verification
### task-2.1 Exercise protocol boundaries
Goal: Prove accepted and rejected callbacks with local provider fixtures.
Files: driver/method/telegram/*_test.go.
Acceptance: Acceptance scenarios, concurrency, no automatic POST retries.
Verification: go test -race ./driver/method/telegram; go test ./...
## Phase 3: Host integration
### task-3.1 Document wiring and recovery
Goal: Show configuration and browser callback association.
Files: driver/method/telegram/example_test.go, docs/telegram-oauth.md, docs/authentication-drivers.md.
Acceptance: Compiled configuration example and explicit host responsibilities.
Verification: go vet ./driver/method/telegram; make linter; compatible scoped golangci-lint run.
# Checkpoints
Only new observable behavior, authority, scope or destructive work requires approval.
# Verification commands
Run each task's commands and record outcomes here after execution.

# Execution results
All three tasks implemented. Added the driver, direct Resty/go-jose dependencies,
protocol tests and compiled host wiring example. No core, schema or generated
contracts changed.

- `GOCACHE=/tmp/telegram-oauth-go-cache go test -race ./driver/method/telegram`: passed, including the compiled example.
- Coverage run: 96.5% of statements; callback, token exchange and ID-token verification each have 100% statement coverage.
- `GOCACHE=/tmp/telegram-oauth-go-cache go vet ./driver/method/telegram`: passed.
- `GOCACHE=/tmp/telegram-oauth-go-cache GOLANGCI_LINT_CACHE=/tmp/telegram-oauth-lint-cache /tmp/telegram-oauth-tools/golangci-lint run --no-config ./driver/method/telegram`: passed (0 issues), using v2.12.2 rebuilt with Go 1.26.0.
- `GOCACHE=/tmp/telegram-oauth-go-cache GOPROXY=off go mod tidy`: passed. Concurrent Goth/logpass changes were preserved.
- `GOCACHE=/tmp/telegram-oauth-go-cache go test ./...`: Telegram and other loadable packages passed; root, application authentication-service and delivery authentication-service tests cannot load the missing `authorization/internal/testutil/authfixture` package.
- `make linter`: unavailable because this checkout lacks `.golangci.yaml`. The scoped no-config run does not substitute for the missing repository lint policy.

HTTP fixture tests needed sandbox escalation for local HTTPS listeners; no live
Telegram credential or provider exchange was used. This directory has no `.git`,
so git status/diff and branch management are unavailable.
