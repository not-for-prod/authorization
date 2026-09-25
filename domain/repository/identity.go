package repository

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Identity stores links from stable issuer/subject pairs to canonical users.
// Create enforces uniqueness of both ID and (Issuer, Subject), returning
// ErrAlreadyExists for either conflict. It must never reassign an existing link
// or merge users based on email. Delete requires a locked Get in the same
// storage transaction; account-linking authorization belongs to the caller.
//
//go:generate moq -out ../../generated/mocks/repository/identity.go -pkg repositorymock . Identity
type Identity interface {
	Create(ctx context.Context, identity entity.Identity) error
	Get(ctx context.Context, id entity.IdentityID, opts ...SelectOption) (*entity.Identity, error)
	GetBySubject(ctx context.Context, issuer, subject string, opts ...SelectOption) (*entity.Identity, error)
	Delete(ctx context.Context, id entity.IdentityID) error
}
