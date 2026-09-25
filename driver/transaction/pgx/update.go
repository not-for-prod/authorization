package pgx

import (
	"context"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/not-for-prod/observer/tracer/prospan"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	"github.com/not-for-prod/authorization/generated/xo"
	"github.com/not-for-prod/authorization/internal/pkg/convert"
)

func (r *Repository) Update(ctx context.Context, value entity.Transaction) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return span.Err(err)
	}
	defer release()
	previous, locked := access.Transactions[postgres.TransactionKey{Tenant: value.TenantID, ID: value.ID}]
	if !locked {
		var id string
		if err = access.DB.QueryRow(ctx, "SELECT "+xo.AuthTransaction_ID+" FROM "+xo.Table_AuthTransaction+" WHERE "+xo.AuthTransaction_TenantID+"=$1 AND "+xo.AuthTransaction_ID+"=$2", string(value.TenantID), string(value.ID)).Scan(&id); err != nil {
			return span.Err(postgres.FromPG(err))
		}
		return span.Err(postgres.Invalid("update requires root lock"))
	}
	if previous.Intent != value.Intent || previous.OwnerKey != value.OwnerKey ||
		!previous.CreatedAt.Equal(value.CreatedAt) ||
		!previous.ExpiresAt.Equal(value.ExpiresAt) ||
		previous.Settings != value.Settings ||
		!reflect.DeepEqual(previous.Policy, value.Copy().Policy) {
		return span.Err(postgres.Invalid("immutable transaction fields changed"))
	}
	row, err := convert.TransactionToXO(value)
	if err != nil {
		return span.Err(err)
	}
	tag, err := access.DB.Exec(
		ctx,
		"UPDATE "+xo.Table_AuthTransaction+" SET "+xo.AuthTransaction_Snapshot+"=$1 WHERE "+xo.AuthTransaction_TenantID+"=$2 AND "+xo.AuthTransaction_ID+"=$3",
		row.Snapshot,
		row.TenantID,
		row.ID,
	)
	if err != nil {
		return span.Err(postgres.FromPG(err))
	}
	if tag.RowsAffected() != 1 {
		return span.Err(postgres.FromPG(pgx.ErrNoRows))
	}
	return nil
}
