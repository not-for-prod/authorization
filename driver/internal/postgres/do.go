package postgres

import (
	"context"
	"errors"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	trm "github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Do holds root locks to commit and rejects nested, foreign and expired scopes.
func (s *Store) Do(ctx context.Context, operation func(context.Context) error) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if ctx.Value(contextKey{}) != nil || trmpgx.DefaultCtxGetter.DefaultTrOrDB(ctx, nil) != nil {
		return span.Err(Invalid("nested storage transaction"))
	}
	if operation == nil {
		return span.Err(Invalid("nil transaction callback"))
	}
	// Mask callback errors from TRM's optional skippable-error behavior: every error must roll back.
	callbackFailed := errors.New("authentication storage callback failed")
	var callbackErr error
	err := s.manager.Do(ctx, func(ctx context.Context) error {
		tx := &scope{owner: s, active: true, access: Access{
			DB:           trmpgx.DefaultCtxGetter.DefaultTrOrDB(ctx, nil),
			Transactions: make(map[TransactionKey]entity.Transaction),
			Users:        make(map[entity.UserID]entity.User),
			Identities:   make(map[entity.IdentityID]bool),
		}}
		defer func() { tx.mu.Lock(); tx.active = false; tx.mu.Unlock() }()
		callbackErr = operation(context.WithValue(ctx, contextKey{}, tx))
		if callbackErr != nil {
			return callbackFailed
		}
		return ctx.Err()
	})
	if errors.Is(err, callbackFailed) {
		if errors.Is(err, trm.ErrRollback) {
			return span.Err(errors.Join(callbackErr, FromPG(err)))
		}
		return span.Err(callbackErr)
	}
	if err != nil {
		return span.Err(FromPG(err))
	}
	return nil
}
