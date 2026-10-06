package redisprogress

import (
	"context"
	"reflect"

	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
)

func (s *Store) Update(ctx context.Context, value entity.Transaction) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	tx, err := s.access(ctx)
	if err != nil {
		return span.Err(err)
	}
	defer tx.mu.Unlock()
	previous := tx.original
	if previous == nil || tx.key != key(value.TenantID, value.ID) {
		return span.Err(invalid("update requires version-checked Get"))
	}
	if previous.Intent != value.Intent || previous.OwnerKey != value.OwnerKey ||
		!previous.CreatedAt.Equal(value.CreatedAt) ||
		!previous.ExpiresAt.Equal(value.ExpiresAt) ||
		previous.Settings != value.Settings ||
		!reflect.DeepEqual(previous.Policy, value.Policy) {
		return span.Err(invalid("immutable transaction fields changed"))
	}
	copy := value.Copy()
	tx.next = &copy
	return nil
}
