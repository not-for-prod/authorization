// Package pgx appends authentication events to the transactional Vibox outbox.
package pgx

import (
	"context"

	"github.com/not-for-prod/vibox"

	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	manager "github.com/not-for-prod/authorization/driver/transactionmanager/pgx"
)

const Topic = "authorization.transactions.v1"

type Publisher struct {
	manager  *postgres.Store
	producer *vibox.Producer
}

var _ repository.EventPublisher = (*Publisher)(nil)

// New loads Vibox topic partitions. Apply migrations before constructing it.
func New(ctx context.Context, m *manager.Manager) (*Publisher, error) {
	if m == nil {
		return nil, postgres.Invalid("transaction manager is required")
	}
	producer, err := m.NewProducer(ctx)
	if err != nil {
		return nil, err
	}
	return &Publisher{manager: m, producer: producer}, nil
}
