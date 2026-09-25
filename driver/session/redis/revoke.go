package redis

import (
	"context"
	"errors"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Revoke is idempotent and retains issuance recovery without reactivating credentials.
func (s *SessionManager) Revoke(ctx context.Context, id entity.SessionID) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if err := ctx.Err(); err != nil {
		return span.Err(err)
	}
	const script = `if redis.call('EXISTS', KEYS[1]) == 1 then redis.call('HSET', KEYS[1], 'revoked', '1') end return 1`
	if err := s.client.Eval(ctx, script, []string{s.prefix + "session:" + digest(string(id))}).Err(); err != nil {
		return span.Err(errors.Join(&proterror.Unavailable{}, err))
	}
	return nil
}
