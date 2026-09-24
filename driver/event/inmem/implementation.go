// Package inmem provides an atomic, non-durable outbox for development.
package inmem

import (
	"authorization/domain/repository"
	manager "authorization/driver/transactionmanager/inmem"
)

type Publisher struct{ manager *manager.Manager }

var _ repository.EventPublisher = (*Publisher)(nil)

func New(manager *manager.Manager) *Publisher { return &Publisher{manager: manager} }
