package inmem

import (
	"context"

	"github.com/not-for-prod/authorization"
)

// Revoke is idempotent, including for session IDs unknown to this manager.
func (s *SessionManager) Revoke(ctx context.Context, id authorization.SessionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	for _, issued := range s.issued {
		if issued.Session.ID == id {
			s.revoked[id] = true
			break
		}
	}

	return nil
}
