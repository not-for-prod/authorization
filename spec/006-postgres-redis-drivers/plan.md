# Implementation phases
Execute in dependency order.
## Tasks
### task-1.1 Schema and generation
Goal: Add storage and Vibox migrations, isolated Compose services, and xo model templates.
Files: tools/migrations, tools/xo-templates, build/docker-compose.yaml, Makefile, generated/xo.
Acceptance: Migration up/down and generated models match schema.
Verification: make migrations-up; make xo; migration integration test.
### task-2.1 PostgreSQL drivers
Goal: Implement shared transaction context, versioned transaction codec, user/identity/transaction repositories and atomic Vibox publisher.
Files: driver/*/pgx, driver/internal/postgres, internal/pkg/convert.
Acceptance: Storage contract, locks, immutability, rollback, uniqueness and event atomicity tests pass.
Verification: PostgreSQL integration tests and codec tests.
### task-3.1 Redis sessions
Goal: Atomic recoverable opaque session manager with tenant isolation and revocation.
Files: driver/session/redis.
Acceptance: Concurrent issuance, recovery, expiry and revocation tests pass.
Verification: Redis integration tests.
### task-4.1 Integration and handoff
Goal: Document host wiring, update storage context, format and verify.
Files: driver/README.md, docs, .golangci.yaml.
Acceptance: Examples compile, complete Go tests and lint pass.
Verification: go test ./...; go test -race on changed packages; make linter; git diff --check.
## Checkpoints
No further routine approval gates. Never reset a user database.
## Verification commands
Use isolated Compose project authorization-drivers with dedicated PostgreSQL and Redis ports. Set AUTHORIZATION_TEST_DATABASE_URL and AUTHORIZATION_TEST_REDIS_ADDR for live integration tests.

## Execution record

- task-1.1: Implemented. Added three Goose migrations, canonical Vibox license,
  PostgreSQL/Redis Compose setup, and xo row/constant templates. `make migrations-up`
  and `make xo` passed against the isolated database. Migration rollback/reapply
  passed in a temporary schema.
- task-2.1: Implemented. Added all PostgreSQL dependency drivers, versioned policy
  snapshot codec, exact owner filtering, immutable-field checks, and Vibox events.
  Real PostgreSQL tests cover locks, cancellation, panic/error rollback, concurrent
  sign-up uniqueness, identity deletion, missing records, and event payloads.
  Race testing found concurrent rollback in Avito's default pgx factory. A local
  factory preserves its default context protocol while finalizing pgx synchronously
  after callback completion, with bounded cancellation-independent rollback.
- task-3.1: Implemented. Redis sessions provide atomic issuance, replay recovery,
  expiry, revocation and tenant isolation. Tests cover concurrent requests, UTC
  principal equivalence, recovery deadlines, retention, malformed credentials,
  cancellation and dependency outages.
- task-4.1: Implementation and focused verification complete. Added wiring docs,
  a compilable example, embedded migrations, `make drivers-test`, and the previously
  missing standard lint/format configuration. Adapted new imports to the concurrent
  module-path change without reverting other edits.

Final verification:

- `make drivers-test`: passed with `-race`, live PostgreSQL and Redis.
- `golangci-lint run` over all new driver, converter, fixture and migration packages:
  passed, zero issues, using `/tmp/authorization-golangci-lint` built with Go 1.26.5.
- `go build ./...`: passed.
- `git diff --check`: passed.
- `go test ./...`: blocked by missing existing `internal/testutil/authfixture` and
  existing Goth callback tests. The Goth failures reproduce using HEAD's original
  dependency versions in a temporary modfile with the current module path.
- `make linter`: blocked by the same missing authfixture import.
- `go mod tidy`: blocked by that missing fixture. `go mod tidy -e` normalized the
  available dependency graph and reported the unresolved test import.

No branch, commit, production deployment, or user-database reset was performed.
The isolated development containers were stopped after verification.
