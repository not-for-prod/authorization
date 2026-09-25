package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"strings"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
)

//go:embed validate.lua
var validateScript string

func (s *SessionManager) Validate(ctx context.Context, token string) (entity.Session, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if err := ctx.Err(); err != nil {
		return entity.Session{}, span.Err(err)
	}
	id, secret, ok := strings.Cut(token, ".")
	if !ok || len(id) != 26 || len(secret) != 26 {
		return entity.Session{}, span.Err(&proterror.Unauthenticated{})
	}
	payload, err := s.client.Eval(ctx, validateScript, []string{s.prefix + "session:" + digest(id)}, digest(token)).
		Text()
	if err != nil {
		return entity.Session{}, span.Err(errors.Join(&proterror.Unavailable{}, err))
	}
	if payload == "" {
		return entity.Session{}, span.Err(&proterror.Unauthenticated{})
	}
	var session entity.Session
	if err = json.Unmarshal([]byte(payload), &session); err != nil {
		return entity.Session{}, span.Err(errors.Join(&proterror.Internal{}, err))
	}
	if session.Principal.TenantID != s.tenant || string(session.ID) != id {
		return entity.Session{}, span.Err(&proterror.Unauthenticated{})
	}
	return session, nil
}
