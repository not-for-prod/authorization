package inmem

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/memory"
)

func (r *Repository) Update(ctx context.Context, row entity.User) error {
	data, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return err
	}
	defer release()

	key := row.ID

	previous, exists := data.Users[key]
	if !exists {
		return memory.Missing()
	}

	if !data.LockedUsers[key] {
		return memory.Invalid("update requires root lock")
	}

	if !previous.CreatedAt.Equal(row.CreatedAt) {
		return memory.Invalid("immutable fields changed")
	}

	data.Users[key] = row

	return nil
}
