package pgx

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	"github.com/not-for-prod/authorization/generated/xo"
	"github.com/not-for-prod/observer/tracer/prospan"
)

// GetAttachedEmail reads the existing address for the locked identity-attachment invariant.
func (r *Repository) GetAttachedEmail(ctx context.Context, user entity.UserID) (string, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, false)
	if err != nil {
		return "", span.Err(err)
	}
	defer release()
	var email string
	query := "SELECT CASE WHEN " + xo.AuthIdentity_Kind + "='email' AND " + xo.AuthIdentity_Issuer + "='atlas/email' THEN " + xo.AuthIdentity_Subject + " ELSE " + xo.AuthIdentity_VerifiedEmail + " END FROM " + xo.Table_AuthIdentity + " WHERE " + xo.AuthIdentity_UserID + "=$1 AND ((" + xo.AuthIdentity_Kind + "='email' AND " + xo.AuthIdentity_Issuer + "='atlas/email') OR (" + xo.AuthIdentity_Kind + "='oauth' AND " + xo.AuthIdentity_VerifiedEmail + "<>'')) ORDER BY CASE WHEN " + xo.AuthIdentity_Kind + "='email' THEN 0 ELSE 1 END, " + xo.AuthIdentity_CreatedAt + ", " + xo.AuthIdentity_ID + " LIMIT 1"
	err = access.DB.QueryRow(ctx, query, string(user)).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", span.Err(postgres.FromPG(err))
	}
	return entity.NormalizeVerifiedEmail(email), nil
}

// UpdateVerifiedEmail changes only metadata while the canonical user is locked.
func (r *Repository) UpdateVerifiedEmail(ctx context.Context, value entity.Identity) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	access, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return span.Err(err)
	}
	defer release()
	if _, ok := access.Users[value.UserID]; !ok {
		return span.Err(postgres.Invalid("identity email requires locked user"))
	}
	tag, err := access.DB.Exec(ctx, "UPDATE "+xo.Table_AuthIdentity+" SET "+xo.AuthIdentity_VerifiedEmail+"=$1 WHERE "+xo.AuthIdentity_ID+"=$2 AND "+xo.AuthIdentity_UserID+"=$3", value.VerifiedEmail, string(value.ID), string(value.UserID))
	if err != nil {
		return span.Err(postgres.FromPG(err))
	}
	if tag.RowsAffected() != 1 {
		return span.Err(postgres.FromPG(pgx.ErrNoRows))
	}
	return nil
}

// ProfileEmails adapts the identity repository's metadata operations.
type ProfileEmails struct{ *Repository }

func (r ProfileEmails) Get(ctx context.Context, user entity.UserID) (string, error) {
	return r.GetAttachedEmail(ctx, user)
}
func (r ProfileEmails) Update(ctx context.Context, value entity.Identity) error {
	return r.UpdateVerifiedEmail(ctx, value)
}
