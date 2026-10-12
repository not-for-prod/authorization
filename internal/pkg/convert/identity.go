package convert

import (
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/generated/xo"
)

func IdentityToXO(i entity.Identity) xo.AuthIdentity {
	return xo.AuthIdentity{
		ID:            string(i.ID),
		UserID:        string(i.UserID),
		Kind:          string(i.Kind),
		Issuer:        i.Issuer,
		Subject:       i.Subject,
		VerifiedEmail: i.VerifiedEmail,
		PasswordHash:  i.PasswordHash,
		CreatedAt:     i.CreatedAt,
	}
}
func IdentityFromXO(i xo.AuthIdentity) entity.Identity {
	return entity.Identity{
		ID:            entity.IdentityID(i.ID),
		UserID:        entity.UserID(i.UserID),
		Kind:          entity.IdentityKind(i.Kind),
		Issuer:        i.Issuer,
		Subject:       i.Subject,
		VerifiedEmail: i.VerifiedEmail,
		PasswordHash:  i.PasswordHash,
		CreatedAt:     i.CreatedAt.UTC(),
	}
}
