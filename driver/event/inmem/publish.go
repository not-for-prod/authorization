package inmem

import (
	"context"
	"reflect"

	"authorization/domain/entity"
	"authorization/driver/internal/memory"
)

func (p *Publisher) Publish(ctx context.Context, events []entity.TransactionEvent) error {
	data, release, err := p.manager.Access(ctx, true)
	if err != nil {
		return err
	}
	defer release()

	for _, event := range events {
		if event.ID == "" || event.AggregateVersion == 0 {
			return memory.Invalid("invalid event")
		}

		duplicate := false

		for _, stored := range data.Events {
			if stored.ID == event.ID {
				if !reflect.DeepEqual(stored, event) {
					return memory.Invalid("conflicting event ID")
				}

				duplicate = true

				break
			}
		}

		if !duplicate {
			data.Events = append(data.Events, event.Copy())
		}
	}

	return nil
}
