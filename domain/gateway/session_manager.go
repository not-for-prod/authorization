package gateway

import (
	"context"
	"time"

	"authorization/domain/entity"
)

// SessionManager is a host-configured session implementation for one tenant.
// It owns token issuance and verification. Callers may Create only after policy
// satisfaction and a valid user binding. Validate must reject expired, revoked,
// or invalid credentials, including credentials belonging to another tenant.
// JWT implementations may use a different persistence strategy from opaque
// sessions; implementing this contract does not require repository.Session.
//
//go:generate moq -out ../../generated/mocks/gateway/session_manager.go -pkg gatewaymock . SessionManager
type SessionManager interface {
	Create(ctx context.Context, req CreateSessionRequest) (entity.IssuedSession, error)
	Validate(ctx context.Context, token string) (entity.Session, error)
	Revoke(ctx context.Context, sessionID entity.SessionID) error
}

// CreateSessionRequest identifies a recoverable issuance. Within a tenant, the
// same Key and Principal must return the same session and credential until
// RecoverUntil. Different principal data for the same key must be rejected.
// Create must honor cancellation and reject issuance after RecoverUntil.
// Implementations must make concurrent calls atomic and never log credentials.
type CreateSessionRequest struct {
	Key          entity.TransactionID
	Principal    entity.Principal
	RecoverUntil time.Time
}
