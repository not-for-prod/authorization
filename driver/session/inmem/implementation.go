// Package inmem provides process-local opaque sessions for development and tests.
// Records remain in memory for the manager's lifetime and are lost on restart.
package inmem

import (
	"slices"
	"sync"
	"time"

	"github.com/not-for-prod/proterror/proterror"

	"authorization"
)

// SessionManager is scoped to one tenant. Construct a separate manager per tenant.
// Tokens are opaque random credentials; this adapter does not issue JWTs.
type SessionManager struct {
	mu       sync.Mutex
	tenant   authorization.TenantID
	lifetime time.Duration
	now      func() time.Time
	issued   map[authorization.TransactionID]authorization.IssuedSession
	tokens   map[string]authorization.TransactionID
	revoked  map[authorization.SessionID]bool
}

var _ authorization.SessionManager = (*SessionManager)(nil)

// NewSessionManager creates an empty session manager with an explicit lifetime.
func NewSessionManager(tenant authorization.TenantID, lifetime time.Duration) (*SessionManager, error) {
	if tenant == "" || lifetime <= 0 {
		return nil, &proterror.InvalidArgument{}
	}

	return &SessionManager{tenant: tenant, lifetime: lifetime, now: time.Now,
		issued: make(map[authorization.TransactionID]authorization.IssuedSession),
		tokens: make(map[string]authorization.TransactionID), revoked: make(map[authorization.SessionID]bool)}, nil
}

func copySession(session authorization.Session) authorization.Session {
	session.Principal.Evidence = slices.Clone(session.Principal.Evidence)
	return session
}
