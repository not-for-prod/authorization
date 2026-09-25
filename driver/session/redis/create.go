package redis

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"slices"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

//go:embed create.lua
var createScript string

func (s *SessionManager) Create(ctx context.Context, req gateway.CreateSessionRequest) (entity.IssuedSession, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if err := ctx.Err(); err != nil {
		return entity.IssuedSession{}, span.Err(err)
	}
	if req.Key == "" || req.Principal.UserID == "" || req.Principal.TenantID != s.tenant || req.RecoverUntil.IsZero() {
		return entity.IssuedSession{}, span.Err(&proterror.InvalidArgument{})
	}
	// Compare instants, not a host's time-zone or monotonic-clock representation.
	req.Principal.AuthenticatedAt = req.Principal.AuthenticatedAt.UTC()
	req.Principal.Evidence = slices.Clone(req.Principal.Evidence)
	for index := range req.Principal.Evidence {
		req.Principal.Evidence[index].VerifiedAt = req.Principal.Evidence[index].VerifiedAt.UTC()
	}
	// Canonical JSON preserves every principal field, including evidence ordering.
	principal, err := json.Marshal(req.Principal)
	if err != nil {
		return entity.IssuedSession{}, span.Err(errors.Join(&proterror.InvalidArgument{}, err))
	}
	now, err := s.client.Time(ctx).Result()
	if err != nil {
		return entity.IssuedSession{}, span.Err(errors.Join(&proterror.Unavailable{}, err))
	}
	now = now.UTC()
	if !now.Before(req.RecoverUntil) {
		return entity.IssuedSession{}, span.Err(&proterror.FailedPrecondition{})
	}
	id := rand.Text()
	issued := entity.IssuedSession{
		Token: id + "." + rand.Text(),
		Session: entity.Session{
			ID:        entity.SessionID(id),
			Principal: req.Principal,
			IssuedAt:  now,
			ExpiresAt: now.Add(s.lifetime),
		},
	}
	payload, err := json.Marshal(issued)
	if err != nil {
		return entity.IssuedSession{}, span.Err(errors.Join(&proterror.InvalidArgument{}, err))
	}
	session, err := json.Marshal(issued.Session)
	if err != nil {
		return entity.IssuedSession{}, span.Err(errors.Join(&proterror.InvalidArgument{}, err))
	}
	result, err := s.client.Eval(ctx, createScript, []string{s.prefix + "issuance:" + digest(string(req.Key)), s.prefix + "session:" + digest(id)}, string(principal), req.RecoverUntil.UnixMilli(), issued.Session.ExpiresAt.UnixMilli(), string(payload), string(session), digest(issued.Token)).
		StringSlice()
	if err != nil {
		return entity.IssuedSession{}, span.Err(errors.Join(&proterror.Unavailable{}, err))
	}
	if len(result) != 2 {
		return entity.IssuedSession{}, span.Err(&proterror.Internal{})
	}
	switch result[0] {
	case "expired", "conflict":
		return entity.IssuedSession{}, span.Err(&proterror.FailedPrecondition{})
	case "ok":
		if err = json.Unmarshal([]byte(result[1]), &issued); err != nil {
			return entity.IssuedSession{}, span.Err(errors.Join(&proterror.Internal{}, err))
		}
		return issued, nil
	default:
		return entity.IssuedSession{}, span.Err(&proterror.Internal{})
	}
}
