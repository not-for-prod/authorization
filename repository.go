package authorization

import "github.com/not-for-prod/authorization/domain/repository"

// TransactionRepository exposes the repository contract for host implementations.
type TransactionRepository = repository.Transaction

// CompletionRepository stores immutable finalization receipts for cross-store recovery.
type CompletionRepository = repository.Completion

// UserRepository exposes the repository contract for host implementations.
type UserRepository = repository.User

// IdentityRepository exposes the repository contract for host implementations.
type IdentityRepository = repository.Identity

// PolicyRepository exposes the repository contract for host implementations.
type PolicyRepository = repository.Policy

// ChallengeRepository exposes the repository contract for host implementations.
type ChallengeRepository = repository.Challenge

// SessionRepository exposes the repository contract for host implementations.
type SessionRepository = repository.Session

// TransactionManager exposes the repository contract for host implementations.
type TransactionManager = repository.TransactionManager

// SelectOption exposes the repository contract for host implementations.
type SelectOption = repository.SelectOption

// State and operation values retain their domain semantics.
const (
	SelectForUpdate = repository.SelectForUpdate
)

// ForUpdate requests a root write guard through TransactionManager.Do.
// Redis uses an atomic snapshot comparison; PostgreSQL holds a row lock.
func ForUpdate() SelectOption { return SelectForUpdate }

// EventPublisher records transitions atomically with authentication state.
type EventPublisher = repository.EventPublisher
