package repository

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Transaction stores authentication transactions, including accumulated evidence.
// Get must match tenant, ID, and ownerKey; a mismatch returns ErrNotFound.
// An empty ownerKey is an exact filter, never a wildcard.
// Update requires a locked Get in the same
// storage transaction and must preserve ID, TenantID, Intent, and CreatedAt.
// It atomically persists all root-owned challenges, claims, evidence, settings,
// policy snapshot, pending attempt, receipts, version, caller association, and completion state.
// Persisted snapshots exclude unpublished Events; the application sends them to EventPublisher. There are no
// independent challenge writes. Expiry, OwnerKey, Settings and Policy are immutable.
// Expiry and user-binding decisions belong to the domain/application layer.
//
//go:generate moq -out ../../generated/mocks/repository/transaction.go -pkg repositorymock . Transaction TransactionManager
type Transaction interface {
	Create(ctx context.Context, transaction entity.Transaction) error
	Get(
		ctx context.Context,
		tenantID entity.TenantID,
		id entity.TransactionID,
		ownerKey string,
		opts ...SelectOption,
	) (*entity.Transaction, error)
	Update(ctx context.Context, transaction entity.Transaction) error
}

// TransactionManager scopes an adapter's database/storage transaction. This is
// distinct from entity.Transaction, which is an authentication transaction.
// Do commits only on nil error and rolls back otherwise. Callers must use the
// supplied context for participating repository operations; locks survive until
// completion. Implementations must reject nesting unless they can preserve the
// outer transaction's atomicity. This does not provide a distributed transaction
// across separately configured stores or permit remote calls while locks are held.
type TransactionManager interface {
	Do(ctx context.Context, operation func(context.Context) error) error
}
