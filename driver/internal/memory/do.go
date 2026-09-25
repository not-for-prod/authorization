package memory

import (
	"context"
	"maps"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Do commits the callback's writes together, or discards them on error, panic,
// or cancellation. Use only its callback context for storage within the callback.
// Nested Transactions are rejected; do not retain the callback context.
func (s *Store) Do(ctx context.Context, operation func(context.Context) error) error {
	if ctx.Value(contextKey{}) != nil {
		return Invalid("nested storage transaction")
	}

	if operation == nil {
		return Invalid("nil transaction callback")
	}

	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	defer func() { <-s.gate }()

	if err := ctx.Err(); err != nil {
		return err
	}

	data := &State{
		Transactions: make(map[TransactionKey]entity.Transaction), Users: maps.Clone(s.data.Users),
		Identities: maps.Clone(s.data.Identities), Subjects: maps.Clone(s.data.Subjects),
		LockedTransactions: make(map[TransactionKey]bool), LockedUsers: make(map[entity.UserID]bool), LockedIdentities: make(map[entity.IdentityID]bool),
	}
	for _, event := range s.data.Events {
		data.Events = append(data.Events, event.Copy())
	}

	for key, row := range s.data.Transactions {
		data.Transactions[key] = row.Copy()
	}

	tx := &scope{owner: s, active: true, data: data}

	defer func() { tx.mu.Lock(); tx.active = false; tx.mu.Unlock() }()

	err := operation(context.WithValue(ctx, contextKey{}, tx))

	tx.mu.Lock()
	defer tx.mu.Unlock()

	tx.active = false

	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	s.data = data

	return nil
}
