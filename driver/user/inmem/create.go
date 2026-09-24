package inmem

import (
	"context"

	"authorization/domain/entity"
	"authorization/driver/internal/memory"
)

func (r *Repository) Create(ctx context.Context, row entity.User) error {
	data, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return err
	}
	defer release()

	key := row.ID
	if _, exists := data.Users[key]; exists {
		return memory.Duplicate()
	}

	data.Users[key] = row

	return nil
}
