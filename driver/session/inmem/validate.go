package inmem

import (
	"context"

	"github.com/not-for-prod/proterror/proterror"

	"authorization"
)

func (s *SessionManager) Validate(ctx context.Context, token string) (authorization.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return authorization.Session{}, err
	}

	key, exists := s.tokens[token]
	if !exists {
		return authorization.Session{}, &proterror.Unauthenticated{}
	}

	session := s.issued[key].Session
	if s.revoked[session.ID] || !s.now().Before(session.ExpiresAt) {
		return authorization.Session{}, &proterror.Unauthenticated{}
	}

	return copySession(session), nil
}
