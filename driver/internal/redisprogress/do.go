package redisprogress

import (
	"context"
	"errors"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/repository"
)

// Do stages one root and commits by comparing the exact previously read snapshot.
// It never retries the callback, which may contain non-repeatable provider I/O.
func (s *Store) Do(ctx context.Context, operation func(context.Context) error) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if ctx.Value(contextKey{}) != nil || operation == nil {
		return span.Err(invalid("nested or nil Redis storage callback"))
	}
	tx := &scope{owner: s, active: true}
	defer func() { tx.mu.Lock(); tx.active = false; tx.mu.Unlock() }()
	if err := operation(context.WithValue(ctx, contextKey{}, tx)); err != nil {
		return span.Err(err)
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.active = false
	if err := ctx.Err(); err != nil {
		return span.Err(err)
	}
	if tx.next == nil {
		return nil
	}
	snapshot, err := encode(*tx.next)
	if err != nil {
		return span.Err(err)
	}
	// Absolute expiry is checked by Redis, and never renewed by updates.
	const commit = `
local now=redis.call('TIME')
if tonumber(ARGV[3])<=tonumber(now[1])*1000+math.floor(tonumber(now[2])/1000) then return -2 end
local current=redis.call('GET',KEYS[1])
if ARGV[1]=='' then
 if current then return -1 end
else
 if not current then return -2 end
 if current~=ARGV[1] then return 0 end
end
redis.call('SET',KEYS[1],ARGV[2],'PXAT',ARGV[3])
return 1`
	result, err := s.client.Eval(ctx, commit, []string{tx.key}, tx.previous, snapshot, tx.next.ExpiresAt.UnixMilli()).
		Int()
	if err != nil {
		return span.Err(fromRedis(err))
	}
	switch result {
	case 1:
		return nil
	case -1:
		return span.Err(errors.Join(&proterror.AlreadyExists{}, repository.ErrAlreadyExists))
	case -2:
		return span.Err(errors.Join(&proterror.NotFound{}, repository.ErrNotFound))
	default:
		return span.Err(errors.Join(&proterror.Aborted{}, errors.New("authentication progress changed concurrently")))
	}
}
