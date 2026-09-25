// Package inmem provides an atomic, non-durable outbox for development.
package inmem

import (
	"github.com/not-for-prod/authorization/domain/repository"
	manager "github.com/not-for-prod/authorization/driver/transactionmanager/inmem"
)

type Publisher struct{ manager *manager.Manager }

var _ repository.EventPublisher = (*Publisher)(nil)

func New(manager *manager.Manager) *Publisher { return &Publisher{manager: manager} }
