package telegram

import (
	"context"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"authorization/domain/entity"
	"authorization/domain/gateway"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

// CallbackInput contains untrusted provider parameters. Supply a value, not a pointer.
// The host must reject repeated query parameters before constructing this input.
// ErrorDescription is intentionally excluded: provider diagnostics may contain secrets.
type CallbackInput struct {
	Code  string
	State string
	Error string
}

func (CallbackInput) AuthenticationInput() {}

func (i *CallbackInput) Verify() error {
	if len(i.State) != encodedRandomLength || len(i.Code) > maxCodeBytes || len(i.Error) > 256 ||
		(i.Code == "") == (i.Error == "") {
		return errors.Join(&proterror.InvalidArgument{}, gateway.ErrProofRejected)
	}
	return nil
}

type tokenResponse struct {
	IDToken string `json:"id_token"`
}
type tokenError struct {
	Error string `json:"error"`
}
type idClaims struct {
	jwt.Claims
	AuthorizedParty string `json:"azp"`
}

// Callback verifies correlation before exchanging a code. The engine owns replay
// prevention; invoking the driver directly does not consume the stored challenge.
func (m *Method) Callback(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
	challenge entity.Challenge,
	input gateway.MethodInput,
) (gateway.VerificationResult, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	proof, ok := input.(CallbackInput)
	if !ok {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.InvalidArgument{}, gateway.ErrProofRejected),
		)
	}
	if err := proof.Verify(); err != nil {
		return gateway.VerificationResult{}, span.Err(err)
	}
	available, err := m.Available(ctx, tx, selection)
	if err != nil {
		return gateway.VerificationResult{}, span.Err(err)
	}
	now := time.Now()
	if !available || challenge.ID == "" || challenge.TransactionID != tx.ID || challenge.Selection != selection ||
		challenge.Status != entity.ChallengePending || challenge.CreatedAt.Before(tx.CreatedAt) || challenge.CreatedAt.After(now) ||
		!now.Before(challenge.ExpiresAt) ||
		challenge.ExpiresAt.After(tx.ExpiresAt) ||
		len(challenge.State) > maxResponseBytes {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected),
		)
	}
	var state challengeState
	if err := json.Unmarshal(challenge.State, &state); err != nil {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected),
		)
	}
	verifier, err := base64.RawURLEncoding.DecodeString(state.Verifier)
	if err != nil || len(verifier) != randomBytes || len(state.Verifier) != encodedRandomLength ||
		state.TenantID != tx.TenantID || state.ClientID != m.config.ClientID || state.RedirectURL != m.config.RedirectURL ||
		subtle.ConstantTimeCompare(
			[]byte(state.State),
			[]byte(proof.State),
		) != 1 || proof.Error != "" {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected),
		)
	}
	token, err := m.exchangeCode(ctx, proof.Code, state.Verifier)
	if err != nil {
		return gateway.VerificationResult{}, span.Err(err)
	}
	subject, err := m.verifyIDToken(ctx, token)
	if err != nil {
		return gateway.VerificationResult{}, span.Err(err)
	}
	return gateway.VerificationResult{
		Identity: &gateway.VerifiedIdentity{Kind: "oauth", Issuer: Issuer, Subject: subject},
		Evidence: entity.Evidence{
			Requirement: selection.Requirement,
			Method:      selection.Method,
			Provider:    selection.Provider,
			VerifiedAt:  time.Now().UTC(),
		},
	}, nil
}

func (m *Method) exchangeCode(ctx context.Context, code, verifier string) (string, error) {
	// OAuth Basic credentials are form-encoded before constructing the header.
	resp, err := m.client.R().SetContext(ctx).
		SetBasicAuth(url.QueryEscape(m.config.ClientID), url.QueryEscape(m.config.ClientSecret)).
		SetFormData(map[string]string{
			"grant_type": "authorization_code", "code": code, "redirect_uri": m.config.RedirectURL,
			"client_id": m.config.ClientID, "code_verifier": verifier,
		}).Post(m.tokenURL)
	if err != nil {
		// Transport/parser errors may contain request or response secrets.
		return "", errors.Join(
			&proterror.Unavailable{},
			errors.New("telegram token exchange failed"),
		)
	}
	if resp.StatusCode() != http.StatusOK {
		var failure tokenError
		if resp.StatusCode() == http.StatusBadRequest &&
			json.Unmarshal(resp.Body(), &failure) == nil &&
			failure.Error == "invalid_grant" {
			return "", errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected)
		}
		return "", errors.Join(
			&proterror.Unavailable{},
			errors.New("telegram token endpoint rejected the request"),
		)
	}
	var result tokenResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil || result.IDToken == "" {
		return "", errors.Join(
			&proterror.Unavailable{},
			errors.New("invalid Telegram token response"),
		)
	}
	return result.IDToken, nil
}

func (m *Method) verifyIDToken(ctx context.Context, raw string) (string, error) {
	token, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil || len(token.Headers) != 1 || token.Headers[0].KeyID == "" {
		return "", errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected)
	}
	resp, err := m.client.R().SetContext(ctx).Get(m.keysURL)
	if err != nil || resp.StatusCode() != http.StatusOK {
		return "", errors.Join(
			&proterror.Unavailable{},
			errors.New("telegram signing keys unavailable"),
		)
	}
	var keys jose.JSONWebKeySet
	if err := json.Unmarshal(resp.Body(), &keys); err != nil || len(keys.Keys) == 0 {
		return "", errors.Join(
			&proterror.Unavailable{},
			errors.New("invalid Telegram signing keys response"),
		)
	}
	matches := keys.Key(token.Headers[0].KeyID)
	if len(matches) != 1 {
		return "", errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected)
	}
	key := matches[0]
	public, ok := key.Key.(*rsa.PublicKey)
	if !ok || !key.Valid() || public.N.BitLen() < 2048 ||
		(key.Use != "" && key.Use != "sig") || (key.Algorithm != "" && key.Algorithm != string(jose.RS256)) {
		return "", errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected)
	}
	var claims idClaims
	if err := token.Claims(public, &claims); err != nil {
		return "", errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected)
	}
	now := time.Now()
	if claims.Expiry == nil || claims.IssuedAt == nil || strings.TrimSpace(claims.Subject) == "" ||
		!now.Before(claims.Expiry.Time()) || !claims.IssuedAt.Time().Before(claims.Expiry.Time()) ||
		(len(claims.Audience) > 1 && claims.AuthorizedParty == "") ||
		(claims.AuthorizedParty != "" && claims.AuthorizedParty != m.config.ClientID) {
		return "", errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected)
	}
	if err := claims.ValidateWithLeeway(
		jwt.Expected{Issuer: Issuer, AnyAudience: jwt.Audience{m.config.ClientID}, Time: now},
		0,
	); err != nil {
		return "", errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected)
	}
	return claims.Subject, nil
}
