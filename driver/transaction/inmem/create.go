package inmem

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/memory"
)

func (r *Repository) Create(ctx context.Context, row entity.Transaction) error {
	data, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return err
	}
	defer release()

	key := memory.TransactionKey{Tenant: row.TenantID, ID: row.ID}
	if _, exists := data.Transactions[key]; exists {
		return memory.Duplicate()
	}

	data.Transactions[key] = row.Copy()

	return nil
}
