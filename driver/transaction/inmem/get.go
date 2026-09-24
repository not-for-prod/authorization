package inmem

import (
	"context"

	"authorization/domain/entity"
	"authorization/domain/repository"
	"authorization/driver/internal/memory"
)

func (r *Repository) Get(ctx context.Context, Tenant entity.TenantID, ID entity.TransactionID, ownerKey string, opts ...repository.SelectOption) (*entity.Transaction, error) {
	data, release, err := r.manager.Access(ctx, false, opts...)
	if err != nil {
		return nil, err
	}
	defer release()

	key := memory.TransactionKey{Tenant: Tenant, ID: ID}

	row, exists := data.Transactions[key]
	if !exists || row.OwnerKey != ownerKey {
		return nil, memory.Missing()
	}

	if len(opts) > 0 {
		data.LockedTransactions[key] = true
	}

	result := row.Copy()

	return &result, nil
}
