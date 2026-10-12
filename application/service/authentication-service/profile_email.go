package authenticationservice

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/proterror/proterror"
)

func checkUpdateCaller(tx entity.Transaction, caller application.Caller) error {
	if tx.Intent == entity.IntentUpdate && (caller.UserID == nil || tx.UserID == nil || *caller.UserID != *tx.UserID) {
		return &proterror.Unauthenticated{}
	}
	return nil
}
func (i *Implementation) resolveLinkProof(ctx context.Context, tx entity.Transaction, caller application.Caller, proof gateway.VerificationResult) (entity.UserID, *entity.VerifiedIdentity, error) {
	if tx.Intent != entity.IntentUpdate {
		return i.resolveVerifiedSubject(ctx, proof)
	}
	if err := checkUpdateCaller(tx, caller); err != nil {
		return "", nil, err
	}
	if proof.Identity == nil || proof.UserID != nil || proof.Identity.Kind != "email" || proof.Identity.Issuer != "atlas/email" {
		return "", nil, entity.ErrInvalidProof
	}
	value := proof.Identity
	return *tx.UserID, &entity.VerifiedIdentity{Kind: value.Kind, Issuer: value.Issuer, Subject: value.Subject}, nil
}
func (i *Implementation) attachEmail(ctx context.Context, tx *entity.Transaction) error {
	if tx.UserID == nil || tx.VerifiedIdentity == nil || i.profileEmails == nil || tx.Settings.IssueSession {
		return &proterror.FailedPrecondition{}
	}
	user, err := i.users.Get(ctx, *tx.UserID, repository.ForUpdate())
	if err != nil {
		return err
	}
	if user == nil || user.ID != *tx.UserID {
		return &proterror.Unauthenticated{}
	}
	if err = i.eligible(ctx, tx.TenantID, *user); err != nil {
		return err
	}
	current, err := i.profileEmails.Get(ctx, user.ID)
	if err != nil {
		return err
	}
	identity, err := user.AttachEmail(*tx.VerifiedIdentity, current, entity.IdentityID(rand.Text()))
	if err != nil {
		return err
	}
	stored, err := i.identities.GetBySubject(ctx, identity.Issuer, identity.Subject)
	if err == nil {
		if stored.UserID != user.ID {
			return &proterror.AlreadyExists{}
		}
		return nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	identity.CreatedAt = i.now().UTC()
	return i.identities.Create(ctx, identity)
}
func (i *Implementation) retainOAuthEmail(ctx context.Context, tx *entity.Transaction, user entity.User) error {
	proof := tx.VerifiedIdentity
	if i.profileEmails == nil || proof == nil || proof.Kind != "oauth" || proof.VerifiedEmail == "" {
		return nil
	}
	locked, err := i.users.Get(ctx, user.ID, repository.ForUpdate())
	if err != nil {
		return err
	}
	if locked == nil || locked.ID != user.ID {
		return entity.ErrInvalidProof
	}
	if err = i.eligible(ctx, tx.TenantID, *locked); err != nil {
		return err
	}
	stored, err := i.identities.GetBySubject(ctx, proof.Issuer, proof.Subject)
	if err != nil {
		return err
	}
	updated, err := locked.RetainVerifiedEmail(*stored, proof.VerifiedEmail)
	if err != nil {
		return err
	}
	return i.profileEmails.Update(ctx, updated)
}
