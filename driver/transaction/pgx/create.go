package pgx

import (
	"context"

	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	"github.com/not-for-prod/authorization/generated/xo"
	"github.com/not-for-prod/authorization/internal/pkg/convert"
)

func (r *Repository) Create(ctx context.Context, value entity.Transaction) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return span.Err(err)
	}
	defer release()
	row, err := convert.TransactionToXO(value)
	if err != nil {
		return span.Err(err)
	}
	_, err = access.DB.Exec(
		ctx,
		"INSERT INTO "+xo.Table_AuthTransaction+" ("+row.Columns()+") VALUES ($1,$2,$3,$4)",
		row.Values()...)
	if err != nil {
		return span.Err(postgres.FromPG(err))
	}
	return nil
}
