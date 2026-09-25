package authorization

import (
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/domain/service"
)

// Sentinel errors preserve errors.Is identity across the public and internal
// boundaries. Drivers return ErrProofRejected only for definitive rejection.
var (
	ErrInvalidTransaction   = entity.ErrInvalidTransaction
	ErrTransactionExpired   = entity.ErrTransactionExpired
	ErrTransactionBusy      = entity.ErrTransactionBusy
	ErrTransactionAbandoned = entity.ErrTransactionAbandoned
	ErrInvalidTransition    = entity.ErrInvalidTransition
	ErrSubjectMismatch      = entity.ErrSubjectMismatch
	ErrAttemptLimit         = entity.ErrAttemptLimit
	ErrInvalidProof         = entity.ErrInvalidProof
	ErrProofRejected        = gateway.ErrProofRejected
	ErrNotFound             = repository.ErrNotFound
	ErrAlreadyExists        = repository.ErrAlreadyExists
	ErrInvalidPolicy        = service.ErrInvalidPolicy
	ErrPolicyMismatch       = service.ErrPolicyMismatch
)
