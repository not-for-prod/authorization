package authorization

import "authorization/domain/entity"

// TransactionID exposes the entity contract for host implementations.
type TransactionID = entity.TransactionID

// TenantID exposes the entity contract for host implementations.
type TenantID = entity.TenantID

// TransactionStatus exposes the entity contract for host implementations.
type TransactionStatus = entity.TransactionStatus

// OperationKind exposes the entity contract for host implementations.
type OperationKind = entity.OperationKind

// TransactionSettings exposes the entity contract for host implementations.
type TransactionSettings = entity.TransactionSettings

// OperationClaim exposes the entity contract for host implementations.
type OperationClaim = entity.OperationClaim

// Transaction exposes the entity contract for host implementations.
type Transaction = entity.Transaction

// Evidence exposes the entity contract for host implementations.
type Evidence = entity.Evidence

// State and operation values retain their domain semantics.
const (
	TransactionActive     = entity.TransactionActive
	TransactionFinalizing = entity.TransactionFinalizing
	TransactionCompleted  = entity.TransactionCompleted
	TransactionAbandoned  = entity.TransactionAbandoned
	OperationStart        = entity.OperationStart
	OperationVerify       = entity.OperationVerify
	OperationCallback     = entity.OperationCallback
)

// MethodAttempt is transaction-owned pending or completed method metadata.
type MethodAttempt = entity.MethodAttempt
