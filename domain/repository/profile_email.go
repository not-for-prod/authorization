package repository

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
)

// ProfileEmail supports identity attachment checks and persists trusted metadata.
// Update requires the owning user to have been locked in the same transaction.
type ProfileEmail interface {
	Get(ctx context.Context, user entity.UserID) (string, error)
	Update(ctx context.Context, identity entity.Identity) error
}
