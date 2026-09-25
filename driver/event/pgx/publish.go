package pgx

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
	"github.com/not-for-prod/vibox"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
)

func (p *Publisher) Publish(ctx context.Context, events []entity.TransactionEvent) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	_, release, err := p.manager.Access(ctx, true)
	if err != nil {
		return span.Err(err)
	}
	defer release()
	records := make([]vibox.Record, 0, len(events))
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return span.Err(errors.Join(&proterror.InvalidArgument{}, err))
		}
		key, err := json.Marshal([2]string{string(event.TenantID), string(event.AggregateID)})
		if err != nil {
			return span.Err(errors.Join(&proterror.InvalidArgument{}, err))
		}
		records = append(records, vibox.Record{Topic: Topic, PartitionKey: key, Payload: payload})
	}
	if err := p.producer.Publish(ctx, records...); err != nil {
		return span.Err(postgres.FromPG(err))
	}
	return nil
}
