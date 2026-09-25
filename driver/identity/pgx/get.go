package pgx

import (
	"context"

	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	"github.com/not-for-prod/authorization/generated/xo"
	"github.com/not-for-prod/authorization/internal/pkg/convert"
)

func (r *Repository) Get(
	ctx context.Context,
	id entity.IdentityID,
	opts ...repository.SelectOption,
) (*entity.Identity, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, false, opts...)
	if err != nil {
		return nil, span.Err(err)
	}
	defer release()
	var row xo.AuthIdentity
	query := "SELECT " + row.Columns() + " FROM " + xo.Table_AuthIdentity + " WHERE " + xo.AuthIdentity_ID + "=$1"
	if len(opts) > 0 {
		query += " FOR UPDATE"
	}
	if err = access.DB.QueryRow(ctx, query, string(id)).Scan(&row.ID, &row.UserID, &row.Kind, &row.Issuer, &row.Subject, &row.PasswordHash, &row.CreatedAt); err != nil {
		return nil, span.Err(postgres.FromPG(err))
	}
	value := convert.IdentityFromXO(row)
	if len(opts) > 0 {
		access.Identities[value.ID] = true
	}
	return &value, nil
}
