package repository

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
)

// User stores canonical users. User IDs are global in the current domain model.
// Update requires a preceding locked Get in the same storage transaction;
// the user's ID and creation time remain immutable.
//
//go:generate moq -out ../../generated/mocks/repository/user.go -pkg repositorymock . User
type User interface {
	Create(ctx context.Context, user entity.User) error
	Get(ctx context.Context, id entity.UserID, opts ...SelectOption) (*entity.User, error)
	Update(ctx context.Context, user entity.User) error
}
