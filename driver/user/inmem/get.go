package inmem

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/driver/internal/memory"
)

func (r *Repository) Get(ctx context.Context, ID entity.UserID, opts ...repository.SelectOption) (*entity.User, error) {
	data, release, err := r.manager.Access(ctx, false, opts...)
	if err != nil {
		return nil, err
	}
	defer release()

	key := ID

	row, exists := data.Users[key]
	if !exists {
		return nil, memory.Missing()
	}

	if len(opts) > 0 {
		data.LockedUsers[key] = true
	}

	result := row

	return &result, nil
}
