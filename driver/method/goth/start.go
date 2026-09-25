package goth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

// StartInput starts an authorization-code flow. Redirect URLs and credentials
// can only be supplied in trusted Config, never through method input.
type StartInput struct{}

func (StartInput) AuthenticationInput() {}

type challengeState struct {
	TenantID      entity.TenantID        `json:"tenant"`
	TransactionID entity.TransactionID   `json:"transaction"`
	ChallengeID   entity.ChallengeID     `json:"challenge"`
	Selection     entity.MethodSelection `json:"selection"`
	ClientID      string                 `json:"client"`
	CallbackURL   string                 `json:"callback"`
	State         string                 `json:"state"`
	Session       string                 `json:"session"`
}

func (m *Method) Start(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
	input gateway.MethodInput,
) (gateway.StartResult, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	valid := false
	switch value := input.(type) {
	case StartInput:
		valid = true
	case *StartInput:
		valid = value != nil
	}
	available, err := m.Available(ctx, tx, selection)
	if err != nil {
		return gateway.StartResult{}, span.Err(err)
	}
	now := m.now().UTC()
	if !valid || !available || tx.ID == "" || tx.Status != entity.TransactionActive ||
		!now.Before(tx.ExpiresAt) {
		return gateway.StartResult{}, span.Err(
			errors.Join(&proterror.InvalidArgument{}, gateway.ErrProofRejected),
		)
	}
	state := rand.Text()
	session, err := m.provider(ctx).BeginAuth(state)
	if err != nil {
		return gateway.StartResult{}, span.Err(
			errors.Join(&proterror.Internal{}, errors.New("cannot begin Google authorization")),
		)
	}
	redirect, err := session.GetAuthURL()
	if err != nil {
		return gateway.StartResult{}, span.Err(
			errors.Join(&proterror.Internal{}, errors.New("cannot create Google redirect")),
		)
	}
	id := entity.ChallengeID(rand.Text())
	payload, err := json.Marshal(
		challengeState{
			TenantID:      tx.TenantID,
			TransactionID: tx.ID,
			ChallengeID:   id,
			Selection:     selection,
			ClientID:      m.config.ClientID,
			CallbackURL:   m.config.CallbackURL,
			State:         state,
			Session:       session.Marshal(),
		},
	)
	if err != nil {
		return gateway.StartResult{}, span.Err(
			errors.Join(&proterror.Internal{}, errors.New("cannot encode Google challenge")),
		)
	}
	expires := now.Add(m.config.ChallengeLifetime)
	if expires.After(tx.ExpiresAt) {
		expires = tx.ExpiresAt
	}
	return gateway.StartResult{
		RedirectURL: redirect,
		Challenge: &entity.Challenge{ID: id, TransactionID: tx.ID, Selection: selection,
			Status: entity.ChallengePending, CreatedAt: now, ExpiresAt: expires, State: payload},
	}, nil
}
