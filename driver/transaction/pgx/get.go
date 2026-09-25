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
	tenant entity.TenantID,
	id entity.TransactionID,
	owner string,
	opts ...repository.SelectOption,
) (*entity.Transaction, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, false, opts...)
	if err != nil {
		return nil, span.Err(err)
	}
	defer release()
	var row xo.AuthTransaction
	query := "SELECT " + row.Columns() + " FROM " + xo.Table_AuthTransaction + " WHERE " + xo.AuthTransaction_TenantID + "=$1 AND " + xo.AuthTransaction_ID + "=$2 AND " + xo.AuthTransaction_OwnerKey + "=$3"
	if len(opts) > 0 {
		query += " FOR UPDATE"
	}
	if err = access.DB.QueryRow(ctx, query, string(tenant), string(id), owner).Scan(&row.TenantID, &row.ID, &row.OwnerKey, &row.Snapshot); err != nil {
		return nil, span.Err(postgres.FromPG(err))
	}
	value, err := convert.TransactionFromXO(row)
	if err != nil {
		return nil, span.Err(err)
	}
	if len(opts) > 0 {
		access.Transactions[postgres.TransactionKey{Tenant: tenant, ID: id}] = value.Copy()
	}
	return &value, nil
}
