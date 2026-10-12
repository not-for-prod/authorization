package authfixture

import (
	"context"
	"sync/atomic"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	eventinmem "github.com/not-for-prod/authorization/driver/event/inmem"
	transactioninmem "github.com/not-for-prod/authorization/driver/transaction/inmem"
	managerinmem "github.com/not-for-prod/authorization/driver/transactionmanager/inmem"
)

type transactionKey struct{}
type writeCount struct{ value int64 }

// Store adds failure injection and committed-write accounting to the shared
// in-memory adapters, which enforce root locks and roll back failed operations.
type Store struct {
	*transactioninmem.Repository
	manager      *managerinmem.Manager
	events       *eventinmem.Publisher
	writes       atomic.Int64
	UpdateError  func(entity.Transaction) error
	PublishError error
}

func newStore(manager *managerinmem.Manager) *Store {
	return &Store{Repository: transactioninmem.New(manager), manager: manager, events: eventinmem.New(manager)}
}

func (s *Store) Do(ctx context.Context, operation func(context.Context) error) error {
	count := &writeCount{}
	err := s.manager.Do(ctx, func(ctx context.Context) error {
		return operation(context.WithValue(ctx, transactionKey{}, count))
	})
	if err == nil {
		s.writes.Add(count.value)
	}
	return err
}

func (s *Store) Create(ctx context.Context, tx entity.Transaction) error {
	if err := s.Repository.Create(ctx, tx); err != nil {
		return err
	}
	s.recordWrite(ctx)
	return nil
}

func (s *Store) Update(ctx context.Context, tx entity.Transaction) error {
	if s.UpdateError != nil {
		if err := s.UpdateError(tx); err != nil {
			return err
		}
	}
	if err := s.Repository.Update(ctx, tx); err != nil {
		return err
	}
	s.recordWrite(ctx)
	return nil
}

func (s *Store) recordWrite(ctx context.Context) {
	count, ok := ctx.Value(transactionKey{}).(*writeCount)
	if !ok {
		panic("fixture write outside transaction")
	}
	count.value++
}

func (s *Store) Publish(ctx context.Context, events []entity.TransactionEvent) error {
	if s.PublishError != nil {
		return s.PublishError
	}
	return s.events.Publish(ctx, events)
}

func (s *Store) Writes() int64 { return s.writes.Load() }

func (s *Store) Events() []entity.TransactionEvent {
	events, err := s.events.List(context.Background())
	if err != nil {
		panic(err)
	}
	return events
}

// AssertOutsideTransaction prevents provider calls under the fixture's lock.
func AssertOutsideTransaction(ctx context.Context) {
	if ctx.Value(transactionKey{}) != nil {
		panic("provider call inside storage transaction")
	}
}

var (
	_ repository.Transaction        = (*Store)(nil)
	_ repository.TransactionManager = (*Store)(nil)
	_ repository.EventPublisher     = (*Store)(nil)
)
