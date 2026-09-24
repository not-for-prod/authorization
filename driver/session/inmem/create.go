package inmem

import (
	"context"
	"crypto/rand"
	"reflect"

	"github.com/not-for-prod/proterror/proterror"

	"authorization"
)

func (s *SessionManager) Create(ctx context.Context, req authorization.CreateSessionRequest) (authorization.IssuedSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return authorization.IssuedSession{}, err
	}

	if req.Key == "" || req.Principal.UserID == "" || req.Principal.TenantID != s.tenant {
		return authorization.IssuedSession{}, &proterror.InvalidArgument{}
	}

	now := s.now().UTC()
	if !now.Before(req.RecoverUntil) {
		return authorization.IssuedSession{}, &proterror.FailedPrecondition{}
	}

	result, exists := s.issued[req.Key]
	if exists && !reflect.DeepEqual(result.Session.Principal, req.Principal) {
		return authorization.IssuedSession{}, &proterror.FailedPrecondition{}
	}

	if !exists {
		result = authorization.IssuedSession{Token: rand.Text(), Session: authorization.Session{
			ID: authorization.SessionID(rand.Text()), Principal: req.Principal, IssuedAt: now, ExpiresAt: now.Add(s.lifetime),
		}}
		result.Session = copySession(result.Session)
		s.issued[req.Key] = result
		s.tokens[result.Token] = req.Key
	}

	result.Session = copySession(result.Session)

	return result, nil
}
