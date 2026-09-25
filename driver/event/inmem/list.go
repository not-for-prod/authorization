package inmem

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
)

// List returns independent snapshots in commit order, for development inspection.
func (p *Publisher) List(ctx context.Context) ([]entity.TransactionEvent, error) {
	data, release, err := p.manager.Access(ctx, false)
	if err != nil {
		return nil, err
	}
	defer release()

	events := make([]entity.TransactionEvent, len(data.Events))
	for j, event := range data.Events {
		events[j] = event.Copy()
	}

	return events, nil
}
