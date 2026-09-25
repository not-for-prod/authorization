package postgres

import (
	"context"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/vibox"
	"go.opentelemetry.io/otel/propagation"
)

// NewProducer joins this store's transactions and carries only standard trace context.
func (s *Store) NewProducer(ctx context.Context) (*vibox.Producer, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	p, err := vibox.NewProducer(ctx, s.manager, vibox.WithProducerPropagator(propagation.TraceContext{}))
	if err != nil {
		return nil, span.Err(FromPG(err))
	}
	return p, nil
}
