package inmem

import (
	"context"
	"reflect"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/memory"
)

func (r *Repository) Update(ctx context.Context, row entity.Transaction) error {
	data, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return err
	}
	defer release()

	key := memory.TransactionKey{Tenant: row.TenantID, ID: row.ID}

	previous, exists := data.Transactions[key]
	if !exists {
		return memory.Missing()
	}

	if !data.LockedTransactions[key] {
		return memory.Invalid("update requires root lock")
	}

	if !previous.CreatedAt.Equal(row.CreatedAt) || previous.Intent != row.Intent || previous.OwnerKey != row.OwnerKey || !previous.ExpiresAt.Equal(row.ExpiresAt) || previous.Settings != row.Settings || !reflect.DeepEqual(previous.Policy, row.Policy) {
		return memory.Invalid("immutable fields changed")
	}

	data.Transactions[key] = row.Copy()

	return nil
}
