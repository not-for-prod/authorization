package telegram

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

// StartInput initiates a browser redirect. Supply a value, not a pointer.
type StartInput struct{}

func (StartInput) AuthenticationInput() {}

func (m *Method) Start(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
	input gateway.MethodInput,
) (gateway.StartResult, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if _, ok := input.(StartInput); !ok {
		return gateway.StartResult{}, span.Err(
			errors.Join(&proterror.InvalidArgument{}, gateway.ErrProofRejected),
		)
	}
	available, err := m.Available(ctx, tx, selection)
	if err != nil {
		return gateway.StartResult{}, span.Err(err)
	}
	if !available {
		return gateway.StartResult{}, span.Err(
			errors.Join(&proterror.FailedPrecondition{}, gateway.ErrProofRejected),
		)
	}
	material := make([]byte, 3*randomBytes)
	if _, err := rand.Read(material); err != nil {
		return gateway.StartResult{}, span.Err(
			errors.Join(&proterror.Internal{}, errors.New("telegram challenge generation failed")),
		)
	}
	state := challengeState{
		State:    base64.RawURLEncoding.EncodeToString(material[:randomBytes]),
		Verifier: base64.RawURLEncoding.EncodeToString(material[randomBytes : 2*randomBytes]),
		TenantID: tx.TenantID, ClientID: m.config.ClientID, RedirectURL: m.config.RedirectURL,
	}
	private, err := json.Marshal(state)
	if err != nil {
		return gateway.StartResult{}, span.Err(
			errors.Join(&proterror.Internal{}, errors.New("telegram challenge encoding failed")),
		)
	}
	now := time.Now().UTC()
	expires := now.Add(m.config.ChallengeLifetime)
	if tx.ExpiresAt.Before(expires) {
		expires = tx.ExpiresAt
	}
	digest := sha256.Sum256([]byte(state.Verifier))
	query := url.Values{
		"client_id":     {m.config.ClientID},
		"redirect_uri":  {m.config.RedirectURL},
		"response_type": {"code"},
		"scope":         {"openid"},
		"state":         {state.State},
		"code_challenge": {
			base64.RawURLEncoding.EncodeToString(digest[:]),
		},
		"code_challenge_method": {"S256"},
	}
	return gateway.StartResult{
		Challenge: &entity.Challenge{
			ID: entity.ChallengeID(
				base64.RawURLEncoding.EncodeToString(material[2*randomBytes:]),
			),
			TransactionID: tx.ID,
			Selection:     selection,
			Status:        entity.ChallengePending,
			CreatedAt:     now,
			ExpiresAt:     expires,
			State:         private,
		},
		RedirectURL: Issuer + "/auth?" + query.Encode(),
	}, nil
}
