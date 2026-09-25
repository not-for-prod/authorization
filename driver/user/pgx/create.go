package pgx

import (
	"context"

	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	"github.com/not-for-prod/authorization/generated/xo"
	"github.com/not-for-prod/authorization/internal/pkg/convert"
)

func (r *Repository) Create(ctx context.Context, value entity.User) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return span.Err(err)
	}
	defer release()
	row := convert.UserToXO(value)
	_, err = access.DB.Exec(
		ctx,
		"INSERT INTO "+xo.Table_AuthUser+" ("+row.Columns()+") VALUES ($1,$2,$3)",
		row.Values()...)
	if err != nil {
		return span.Err(postgres.FromPG(err))
	}
	return nil
}
