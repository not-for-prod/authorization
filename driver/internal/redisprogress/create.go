package redisprogress

import (
	"context"

	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
)

func (s *Store) Create(ctx context.Context, value entity.Transaction) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	tx, err := s.access(ctx)
	if err != nil {
		return span.Err(err)
	}
	defer tx.mu.Unlock()
	if tx.key != "" {
		return span.Err(invalid("Redis scope already has a root"))
	}
	if value.ID == "" || value.TenantID == "" || value.OwnerKey == "" {
		return span.Err(invalid("missing transaction identity"))
	}
	tx.key = key(value.TenantID, value.ID)
	copy := value.Copy()
	tx.next = &copy
	return nil
}
