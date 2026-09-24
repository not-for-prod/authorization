package inmem

import (
	"context"

	"authorization/domain/entity"
	"authorization/domain/repository"
	"authorization/driver/internal/memory"
)

func (r *Repository) Get(ctx context.Context, ID entity.IdentityID, opts ...repository.SelectOption) (*entity.Identity, error) {
	data, release, err := r.manager.Access(ctx, false, opts...)
	if err != nil {
		return nil, err
	}
	defer release()

	key := ID

	row, exists := data.Identities[key]
	if !exists {
		return nil, memory.Missing()
	}

	if len(opts) > 0 {
		data.LockedIdentities[key] = true
	}

	result := row

	return &result, nil
}
