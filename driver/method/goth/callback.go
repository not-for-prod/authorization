package goth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
	"golang.org/x/oauth2"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

// CallbackInput contains only provider callback fields. The host must reject
// duplicate query parameters and recover the original caller/attempt association.
// Error is Google's OAuth error code; descriptions are deliberately omitted.
type CallbackInput struct {
	Code  string
	State string
	Error string
}

func (CallbackInput) AuthenticationInput() {}

// Verify checks the independently supplied callback fields. State correlation
// and provider denial are checked against the stored challenge by Callback.
func (input *CallbackInput) Verify() error {
	if input == nil || input.State == "" || len(input.State) > 256 || len(input.Code) > 8192 ||
		len(input.Error) > 256 ||
		(strings.TrimSpace(input.Code) == "") == (input.Error == "") {
		return errors.Join(&proterror.InvalidArgument{}, gateway.ErrProofRejected)
	}
	return nil
}

func (m *Method) Callback(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
	challenge entity.Challenge,
	input gateway.MethodInput,
) (gateway.VerificationResult, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	var callback CallbackInput
	switch value := input.(type) {
	case CallbackInput:
		callback = value
	case *CallbackInput:
		if value == nil {
			return gateway.VerificationResult{}, span.Err(errors.Join(&proterror.InvalidArgument{}, gateway.ErrProofRejected))
		}
		callback = *value
	default:
		return gateway.VerificationResult{}, span.Err(errors.Join(&proterror.InvalidArgument{}, gateway.ErrProofRejected))
	}
	if err := callback.Verify(); err != nil {
		return gateway.VerificationResult{}, span.Err(err)
	}
	available, err := m.Available(ctx, tx, selection)
	if err != nil {
		return gateway.VerificationResult{}, span.Err(err)
	}
	now := m.now().UTC()
	if !available || tx.Status != entity.TransactionActive || !now.Before(tx.ExpiresAt) || challenge.ID == "" || challenge.TransactionID != tx.ID || challenge.Selection != selection ||
		challenge.Status != entity.ChallengePending ||
		!now.Before(challenge.ExpiresAt) ||
		challenge.ExpiresAt.After(tx.ExpiresAt) ||
		challenge.CreatedAt.After(now) ||
		len(challenge.State) > 16384 {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected),
		)
	}
	var saved challengeState
	if json.Unmarshal(challenge.State, &saved) != nil || saved.TenantID != tx.TenantID || saved.TransactionID != tx.ID || saved.ChallengeID != challenge.ID || saved.Selection != selection ||
		saved.ClientID != m.config.ClientID ||
		saved.CallbackURL != m.config.CallbackURL ||
		saved.State == "" ||
		subtle.ConstantTimeCompare([]byte(saved.State), []byte(callback.State)) != 1 {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected),
		)
	}
	if callback.Error != "" {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected),
		)
	}
	ctx, cancel := context.WithTimeout(ctx, m.config.Timeout)
	defer cancel()
	provider := m.provider(ctx)
	session, err := provider.UnmarshalSession(saved.Session)
	if err != nil {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Internal{}, errors.New("invalid stored Google session")),
		)
	}
	// The stored session must be the token-free session produced by Start.
	original, err := provider.BeginAuth(saved.State)
	if err != nil || session.Marshal() != original.Marshal() {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Internal{}, errors.New("invalid stored Google session")),
		)
	}
	if _, err := session.Authorize(provider, url.Values{"code": {callback.Code}}); err != nil {
		var rejected *oauth2.RetrieveError
		if errors.As(err, &rejected) && rejected.ErrorCode == "invalid_grant" {
			return gateway.VerificationResult{}, span.Err(
				errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected),
			)
		}
		// OAuth errors can contain response bodies, codes and tokens. Never retain them.
		return gateway.VerificationResult{}, span.Err(
			errors.Join(
				&proterror.Unavailable{},
				errors.New("failed to exchange Google authorization code"),
				ctx.Err(),
			),
		)
	}
	user, err := provider.FetchUser(session)
	if err != nil {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(
				&proterror.Unavailable{},
				errors.New("failed to fetch Google identity"),
				ctx.Err(),
			),
		)
	}
	if err := ctx.Err(); err != nil {
		return gateway.VerificationResult{}, span.Err(err)
	}
	if strings.TrimSpace(user.UserID) == "" || user.Provider != string(Google) {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected),
		)
	}
	return gateway.VerificationResult{
		Evidence: entity.Evidence{
			Requirement: selection.Requirement,
			Method:      selection.Method,
			Provider:    selection.Provider,
			VerifiedAt:  m.now().UTC(),
		},
		Identity: &gateway.VerifiedIdentity{
			Kind:    "oauth",
			Issuer:  GoogleIssuer,
			Subject: user.UserID,
		},
	}, nil
}
