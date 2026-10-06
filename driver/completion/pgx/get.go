package pgx

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"

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
) (*entity.CompletionReceipt, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, false, opts...)
	if err != nil {
		return nil, span.Err(err)
	}
	defer release()
	if len(opts) > 0 {
		// A transaction-scoped advisory lock also protects creation when no row exists.
		encoded, _ := json.Marshal([2]string{string(tenant), string(id)})
		hash := sha256.Sum256(encoded)
		lockID := int64(binary.BigEndian.Uint64(hash[:8]))
		if _, err = access.DB.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockID); err != nil {
			return nil, span.Err(postgres.FromPG(err))
		}
		access.Completions[postgres.TransactionKey{Tenant: tenant, ID: id}] = true
	}
	var row xo.AuthCompletion
	query := "SELECT " + row.Columns() + " FROM " + xo.Table_AuthCompletion + " WHERE " + xo.AuthCompletion_TenantID + "=$1 AND " + xo.AuthCompletion_ID + "=$2 AND " + xo.AuthCompletion_OwnerKey + "=$3"
	if err = access.DB.QueryRow(ctx, query, string(tenant), string(id), owner).
		Scan(&row.TenantID, &row.ID, &row.OwnerKey, &row.Receipt); err != nil {
		return nil, span.Err(postgres.FromPG(err))
	}
	value, err := convert.CompletionFromXO(row)
	if err != nil {
		return nil, span.Err(err)
	}
	return &value, nil
}
