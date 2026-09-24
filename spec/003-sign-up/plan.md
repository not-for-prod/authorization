# Implementation phases

## Tasks
- [x] Restore existing application methods and runtime integration tests.
- [x] Use a shared private verified identity and intent-independent acceptance/binding transition.
- [x] Route satisfied SIGN_UP policies to atomic user/identity creation at finalization.
- [x] Test rollback, concurrency, subject consistency and session recovery.
- [x] Update the current context and visual model for implementation-first review.

## Checkpoints
The owner reviews and corrects SIGN_UP before moving to another flow. No UPDATE
or DELETE behavior is inferred from this implementation.

## Verification commands
- GOCACHE=/tmp/authorization-go-cache go test ./...
- GOCACHE=/tmp/authorization-go-cache go test -race ./application/service/authentication-service ./domain/entity
- Rebuilt golangci-lint run --no-config ./...
- make linter remains blocked by the missing .golangci.yaml.
