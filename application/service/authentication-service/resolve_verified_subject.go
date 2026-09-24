package authenticationservice

import (
	"authorization/domain/entity"
	"authorization/domain/gateway"
	"authorization/domain/repository"
	"context"
	"errors"
)

// resolveVerifiedSubject resolves ownership without applying an intent's completion rules.
func (i *Implementation) resolveVerifiedSubject(ctx context.Context, proof gateway.VerificationResult) (entity.UserID, *entity.VerifiedIdentity, error) {
	if (proof.Identity == nil) == (proof.UserID == nil) {
		return "", nil, entity.ErrInvalidProof
	}
	var userID entity.UserID
	var identity *entity.VerifiedIdentity
	if proof.Identity != nil {
		value := proof.Identity
		if value.Kind == "" || value.Issuer == "" || value.Subject == "" {
			return "", nil, entity.ErrInvalidProof
		}
		identity = &entity.VerifiedIdentity{Kind: value.Kind, Issuer: value.Issuer, Subject: value.Subject}
		stored, err := i.identities.GetBySubject(ctx, value.Issuer, value.Subject)
		if errors.Is(err, repository.ErrNotFound) {
			return "", identity, nil
		}
		if err != nil {
			return "", nil, err
		}
		if stored == nil || stored.Kind != value.Kind || stored.Issuer != value.Issuer || stored.Subject != value.Subject {
			return "", nil, entity.ErrInvalidProof
		}
		userID = stored.UserID
	} else {
		userID = *proof.UserID
	}
	if userID == "" {
		return "", nil, entity.ErrInvalidProof
	}
	user, err := i.users.Get(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	if user == nil || user.ID != userID {
		return "", nil, entity.ErrInvalidProof
	}
	return userID, identity, nil
}
