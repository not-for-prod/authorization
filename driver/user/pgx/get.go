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

func (r *Repository) Get(ctx context.Context, id entity.UserID, opts ...repository.SelectOption) (*entity.User, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, false, opts...)
	if err != nil {
		return nil, span.Err(err)
	}
	defer release()
	var row xo.AuthUser
	query := "SELECT " + row.Columns() + " FROM " + xo.Table_AuthUser + " WHERE " + xo.AuthUser_ID + "=$1"
	if len(opts) > 0 {
		query += " FOR UPDATE"
	}
	if err = access.DB.QueryRow(ctx, query, string(id)).Scan(&row.ID, &row.Status, &row.CreatedAt); err != nil {
		return nil, span.Err(postgres.FromPG(err))
	}
	value := convert.UserFromXO(row)
	if len(opts) > 0 {
		access.Users[value.ID] = value
	}
	return &value, nil
}
