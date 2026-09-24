package repository

import (
	"context"

	"authorization/domain/entity"
)

// EventPublisher atomically records events in the transaction used to persist their aggregate.
// Production adapters must use a durable outbox. A successful return alone is not delivery.
type EventPublisher interface {
	Publish(context.Context, []entity.TransactionEvent) error
}
