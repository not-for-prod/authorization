package repository

import (
	"context"

	"authorization/domain/entity"
)

// Session stores logical sessions, scoped by Principal.TenantID and session ID.
// Token generation, token-to-session lookup, validation, and revocation policy
// belong to a SessionManager adapter. Delete requires a locked Get in the same
// storage transaction. No raw credential is part of this repository contract.
//
//go:generate moq -out ../../generated/mocks/repository/session.go -pkg repositorymock . Session
type Session interface {
	Create(ctx context.Context, session entity.Session) error
	Get(
		ctx context.Context,
		tenantID entity.TenantID,
		id entity.SessionID,
		opts ...SelectOption,
	) (*entity.Session, error)
	Delete(ctx context.Context, tenantID entity.TenantID, id entity.SessionID) error
}
