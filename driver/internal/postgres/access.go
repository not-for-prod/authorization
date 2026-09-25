package postgres

import (
	"context"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"

	"github.com/not-for-prod/authorization/domain/repository"
)

// Access serializes pgx operations in a scope and verifies their ownership.
func (s *Store) Access(ctx context.Context, mutation bool, opts ...repository.SelectOption) (*Access, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	for _, opt := range opts {
		if opt != repository.SelectForUpdate {
			return nil, nil, Invalid("unsupported read option")
		}
	}
	if tx, ok := ctx.Value(contextKey{}).(*scope); ok {
		tx.mu.Lock()
		if tx.owner != s || !tx.active || trmpgx.DefaultCtxGetter.DefaultTrOrDB(ctx, nil) != tx.access.DB {
			tx.mu.Unlock()
			return nil, nil, Invalid("foreign or closed transaction context")
		}
		return &tx.access, tx.mu.Unlock, nil
	}
	if mutation || len(opts) > 0 || trmpgx.DefaultCtxGetter.DefaultTrOrDB(ctx, nil) != nil {
		return nil, nil, Invalid("operation requires this manager's transaction context")
	}
	return &Access{DB: s.pool}, func() {}, nil
}
