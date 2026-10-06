package repository

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Completion stores immutable finalization receipts in the same storage transaction
// as account creation and completion events. A ForUpdate Get serializes the key
// even when absent; Create requires that reservation in the same scope.
type Completion interface {
	Get(
		ctx context.Context,
		tenant entity.TenantID,
		id entity.TransactionID,
		owner string,
		opts ...SelectOption,
	) (*entity.CompletionReceipt, error)
	Create(ctx context.Context, receipt entity.CompletionReceipt) error
}
