package logpass

import (
	"context"
	"errors"
	"time"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
)

// VerifyInput contains transient credentials. Pass a value, not a pointer.
// Neither login nor password is normalized, logged or retained by the driver.
type VerifyInput struct {
	Login    string
	Password string
}

func (VerifyInput) AuthenticationInput() {}

func (m *Method) Verify(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
	challenge *entity.Challenge,
	input gateway.MethodInput,
) (gateway.VerificationResult, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	available, err := m.Available(ctx, tx, selection)
	if err != nil {
		return gateway.VerificationResult{}, span.Err(err)
	}
	proof, valid := input.(VerifyInput)
	if !available || !valid || challenge != nil || len(proof.Login) == 0 || len(proof.Login) > MaxLoginBytes ||
		len(proof.Password) == 0 ||
		len(proof.Password) > MaxPasswordBytes {
		return gateway.VerificationResult{}, span.Err(gateway.ErrProofRejected)
	}

	identity, err := m.identities.GetBySubject(ctx, m.config.Issuer, proof.Login)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return gateway.VerificationResult{}, span.Err(ctxErr)
	}
	missing := errors.Is(err, repository.ErrNotFound)
	if err != nil && !missing {
		// Repository errors can contain query arguments. Never retain the raw cause.
		return gateway.VerificationResult{}, span.Err(errors.Join(
			&proterror.Unavailable{}, errors.New("password identity lookup failed"),
		))
	}
	if !missing &&
		(identity == nil || identity.UserID == "" || identity.Issuer != m.config.Issuer ||
			identity.Subject != proof.Login || len(identity.PasswordHash) > MaxHashBytes) {
		return gateway.VerificationResult{}, span.Err(errors.Join(
			&proterror.Internal{}, errors.New("invalid password identity record"),
		))
	}

	hash := m.config.DummyHash
	enrolled := !missing && identity.PasswordHash != ""
	if enrolled {
		hash = identity.PasswordHash
	}
	matched, err := m.config.VerifyPassword(ctx, hash, proof.Password)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return gateway.VerificationResult{}, span.Err(ctxErr)
	}
	if err != nil {
		return gateway.VerificationResult{}, span.Err(errors.Join(
			&proterror.Internal{}, errors.New("password hash verification failed"),
		))
	}
	if !enrolled || !matched || (tx.UserID != nil && *tx.UserID != identity.UserID) {
		return gateway.VerificationResult{}, span.Err(gateway.ErrProofRejected)
	}

	userID := identity.UserID
	return gateway.VerificationResult{
		UserID: &userID,
		Evidence: entity.Evidence{
			Requirement: selection.Requirement, Method: selection.Method,
			Provider: selection.Provider, VerifiedAt: time.Now().UTC(),
		},
	}, nil
}
