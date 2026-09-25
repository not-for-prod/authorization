package pgx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	"github.com/not-for-prod/authorization/generated/xo"
)

func (r *Repository) Delete(ctx context.Context, id entity.IdentityID) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return span.Err(err)
	}
	defer release()
	if !access.Identities[id] {
		var existing string
		if err = access.DB.QueryRow(ctx, "SELECT "+xo.AuthIdentity_ID+" FROM "+xo.Table_AuthIdentity+" WHERE "+xo.AuthIdentity_ID+"=$1", string(id)).Scan(&existing); err != nil {
			return span.Err(postgres.FromPG(err))
		}
		return span.Err(postgres.Invalid("delete requires root lock"))
	}
	tag, err := access.DB.Exec(ctx, "DELETE FROM "+xo.Table_AuthIdentity+" WHERE "+xo.AuthIdentity_ID+"=$1", string(id))
	if err != nil {
		return span.Err(postgres.FromPG(err))
	}
	if tag.RowsAffected() != 1 {
		return span.Err(postgres.FromPG(pgx.ErrNoRows))
	}
	delete(access.Identities, id)
	return nil
}
