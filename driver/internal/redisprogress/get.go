package redisprogress

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
	goredis "github.com/redis/go-redis/v9"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/generated/xo"
	"github.com/not-for-prod/authorization/internal/pkg/convert"
)

func (s *Store) Get(
	ctx context.Context,
	tenant entity.TenantID,
	id entity.TransactionID,
	owner string,
	opts ...repository.SelectOption,
) (*entity.Transaction, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	for _, opt := range opts {
		if opt != repository.SelectForUpdate {
			return nil, span.Err(invalid("unsupported read option"))
		}
	}
	var tx *scope
	if len(opts) > 0 || ctx.Value(contextKey{}) != nil {
		var err error
		tx, err = s.access(ctx)
		if err != nil {
			return nil, span.Err(err)
		}
		defer tx.mu.Unlock()
		if tx.key != "" && tx.key != key(tenant, id) {
			return nil, span.Err(invalid("Redis scope supports one root"))
		}
		if tx.next != nil {
			value := tx.next.Copy()
			if value.OwnerKey != owner {
				return nil, span.Err(fromRedis(goredis.Nil))
			}
			return &value, nil
		}
	}
	raw, err := s.client.Get(ctx, key(tenant, id)).Result()
	if err != nil {
		return nil, span.Err(fromRedis(err))
	}
	// Owner is checked against the envelope by the common snapshot decoder.
	// Decode stored state before checking ownership to keep mismatches NotFound.
	value, err := decode(raw, tenant, id)
	if err != nil {
		return nil, span.Err(err)
	}
	if value.OwnerKey != owner {
		return nil, span.Err(fromRedis(goredis.Nil))
	}
	if tx != nil && len(opts) > 0 {
		tx.key = key(tenant, id)
		tx.previous = raw
		copy := value.Copy()
		tx.original = &copy
	}
	return &value, nil
}

func decode(raw string, tenant entity.TenantID, id entity.TransactionID) (entity.Transaction, error) {
	// The envelope includes the exact owner; the outer adapter applies its filter.
	var envelope struct{ State struct{ OwnerKey string } }
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return entity.Transaction{}, errors.Join(&proterror.Internal{}, err)
	}
	return convert.TransactionFromXO(
		xo.AuthTransaction{
			TenantID: string(tenant),
			ID:       string(id),
			OwnerKey: envelope.State.OwnerKey,
			Snapshot: []byte(raw),
		},
	)
}
