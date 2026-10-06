package pgx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	"github.com/not-for-prod/authorization/generated/xo"
)

// Delete removes the canonical user and its owned login identities atomically.
func (r *Repository) Delete(ctx context.Context, id entity.UserID) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return span.Err(err)
	}
	defer release()
	if _, locked := access.Users[id]; !locked {
		return span.Err(postgres.Invalid("delete requires root lock"))
	}
	if _, err = access.DB.Exec(ctx, "DELETE FROM "+xo.Table_AuthIdentity+" WHERE "+xo.AuthIdentity_UserID+"=$1", string(id)); err != nil {
		return span.Err(postgres.FromPG(err))
	}
	tag, err := access.DB.Exec(ctx, "DELETE FROM "+xo.Table_AuthUser+" WHERE "+xo.AuthUser_ID+"=$1", string(id))
	if err != nil {
		return span.Err(postgres.FromPG(err))
	}
	if tag.RowsAffected() != 1 {
		return span.Err(postgres.FromPG(pgx.ErrNoRows))
	}
	delete(access.Users, id)
	return nil
}
