package pgx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	"github.com/not-for-prod/authorization/generated/xo"
)

func (r *Repository) Update(ctx context.Context, value entity.User) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return span.Err(err)
	}
	defer release()
	previous, locked := access.Users[value.ID]
	if !locked {
		var id string
		if err = access.DB.QueryRow(ctx, "SELECT "+xo.AuthUser_ID+" FROM "+xo.Table_AuthUser+" WHERE "+xo.AuthUser_ID+"=$1", string(value.ID)).Scan(&id); err != nil {
			return span.Err(postgres.FromPG(err))
		}
		return span.Err(postgres.Invalid("update requires root lock"))
	}
	if !previous.CreatedAt.Equal(value.CreatedAt) {
		return span.Err(postgres.Invalid("immutable user fields changed"))
	}
	tag, err := access.DB.Exec(
		ctx,
		"UPDATE "+xo.Table_AuthUser+" SET "+xo.AuthUser_Status+"=$1 WHERE "+xo.AuthUser_ID+"=$2",
		string(value.Status),
		string(value.ID),
	)
	if err != nil {
		return span.Err(postgres.FromPG(err))
	}
	if tag.RowsAffected() != 1 {
		return span.Err(postgres.FromPG(pgx.ErrNoRows))
	}
	return nil
}
