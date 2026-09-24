package memory

import (
	"context"

	"authorization/domain/repository"
)

// Access holds either the transaction scope or store lock until release. Escaped
// callback contexts cannot read or mutate committed data after Do returns.
func (s *Store) Access(ctx context.Context, mutation bool, opts ...repository.SelectOption) (*State, func(), error) {
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

		if tx.owner != s || !tx.active {
			tx.mu.Unlock()
			return nil, nil, Invalid("foreign or closed transaction context")
		}

		return tx.data, tx.mu.Unlock, nil
	}

	if mutation || len(opts) != 0 {
		return nil, nil, Invalid("mutation or locked read requires transaction context")
	}

	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		<-s.gate
		return nil, nil, err
	}

	return s.data, func() { <-s.gate }, nil
}
