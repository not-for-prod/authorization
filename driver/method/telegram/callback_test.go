package telegram

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/not-for-prod/proterror/proterror"
)

type providerFixture struct {
	method      *Method
	tx          entity.Transaction
	selection   entity.MethodSelection
	started     gateway.StartResult
	input       CallbackInput
	private     *rsa.PrivateKey
	token       string
	keys        []byte
	tokenStatus int
	keysStatus  int
	tokenBody   string
	keyBody     string
	redirect    string
	delay       time.Duration
	posts       atomic.Int32
	gets        atomic.Int32
}

func newProviderFixture(t *testing.T) *providerFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	method, err := New(
		Config{
			ClientID:     "123456",
			ClientSecret: "secret:&+",
			RedirectURL:  "https://host.example/auth/telegram",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f := &providerFixture{
		method:      method,
		private:     key,
		tokenStatus: http.StatusOK,
		keysStatus:  http.StatusOK,
		tx: entity.Transaction{
			ID:        "transaction",
			TenantID:  "tenant",
			Status:    entity.TransactionActive,
			CreatedAt: time.Now().Add(-time.Minute),
			ExpiresAt: time.Now().Add(time.Hour),
		},
		selection: entity.MethodSelection{
			Requirement: "primary",
			Method:      "oauth",
			Provider:    Provider,
		},
	}
	f.keys, err = json.Marshal(
		jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{
				{Key: &key.PublicKey, KeyID: "telegram-key", Algorithm: "RS256", Use: "sig"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.token = f.sign(t, f.claims())
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			f.posts.Add(1)
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			username, password, ok := r.BasicAuth()
			if !ok || username != url.QueryEscape(f.method.config.ClientID) ||
				password != url.QueryEscape(f.method.config.ClientSecret) {
				t.Error("incorrect Basic authentication")
			}
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			var state challengeState
			if err := json.Unmarshal(f.started.Challenge.State, &state); err != nil {
				t.Error(err)
			}
			expected := url.Values{
				"grant_type":    {"authorization_code"},
				"code":          {"provider-code"},
				"client_id":     {f.method.config.ClientID},
				"redirect_uri":  {f.method.config.RedirectURL},
				"code_verifier": {state.Verifier},
			}
			if r.PostForm.Encode() != expected.Encode() {
				t.Error("incorrect code exchange form")
			}
			if f.delay > 0 {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(f.delay):
				}
			}
			if f.redirect != "" {
				w.Header().Set("Location", f.redirect)
				w.WriteHeader(http.StatusTemporaryRedirect)
				return
			}
			w.WriteHeader(f.tokenStatus)
			if f.tokenBody != "" {
				_, _ = fmt.Fprint(w, f.tokenBody)
				return
			}
			if err := json.NewEncoder(w).Encode(tokenResponse{IDToken: f.token}); err != nil {
				t.Error(err)
			}
		case "/keys":
			f.gets.Add(1)
			if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" {
				t.Error("keys request leaked authorization or used incorrect method")
			}
			w.WriteHeader(f.keysStatus)
			if f.keyBody != "" {
				_, _ = fmt.Fprint(w, f.keyBody)
				return
			}
			_, _ = w.Write(f.keys)
		default:
			t.Error("unexpected provider endpoint")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	f.method.client.SetTransport(server.Client().Transport)
	f.method.tokenURL = server.URL + "/token"
	f.method.keysURL = server.URL + "/keys"
	f.started, err = f.method.Start(t.Context(), f.tx, f.selection, StartInput{})
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := url.Parse(f.started.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	f.input = CallbackInput{Code: "provider-code", State: redirect.Query().Get("state")}
	return f
}

func (f *providerFixture) claims() idClaims {
	return idClaims{
		Claims: jwt.Claims{
			Issuer:   Issuer,
			Subject:  "oidc-subject",
			Audience: jwt.Audience{f.method.config.ClientID},
			IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

func (f *providerFixture) sign(t *testing.T, claims idClaims) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: f.private},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "telegram-key"),
	)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestStartAndCallback(t *testing.T) {
	f := newProviderFixture(t)
	redirect, err := url.Parse(f.started.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	var state challengeState
	if err := json.Unmarshal(f.started.Challenge.State, &state); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(state.Verifier))
	expected := url.Values{
		"client_id":             {"123456"},
		"redirect_uri":          {f.method.config.RedirectURL},
		"response_type":         {"code"},
		"scope":                 {"openid"},
		"state":                 {state.State},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(digest[:])},
		"code_challenge_method": {"S256"},
	}
	if redirect.Scheme+"://"+redirect.Host+redirect.Path != Issuer+"/auth" ||
		redirect.Query().Encode() != expected.Encode() {
		t.Fatal("invalid authorization URL")
	}
	if state.State == state.Verifier || strings.Contains(f.started.RedirectURL, state.Verifier) ||
		strings.Contains(f.started.RedirectURL, f.method.config.ClientSecret) {
		t.Fatal("redirect leaks private material")
	}
	if f.started.Challenge.ExpiresAt.Sub(f.started.Challenge.CreatedAt) != defaultLifetime {
		t.Fatal("incorrect challenge lifetime")
	}
	result, err := f.method.Callback(t.Context(), f.tx, f.selection, *f.started.Challenge, f.input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity == nil ||
		*result.Identity != (gateway.VerifiedIdentity{Kind: "oauth", Issuer: Issuer, Subject: "oidc-subject"}) ||
		result.UserID != nil {
		t.Fatalf("invalid identity result: %+v", result)
	}
	if result.Evidence.Method != f.selection.Method ||
		result.Evidence.Provider != f.selection.Provider ||
		result.Evidence.Requirement != f.selection.Requirement ||
		result.Evidence.VerifiedAt.IsZero() {
		t.Fatal("incorrect evidence")
	}
	if f.posts.Load() != 1 || f.gets.Load() != 1 {
		t.Fatal("unexpected request counts")
	}
	if f.started.Challenge.Status != entity.ChallengePending || f.tx.UserID != nil {
		t.Fatal("driver mutated engine state")
	}
	// Verify uses precisely the same checks, with consumption still owned by the engine.
	if _, err := f.method.Verify(
		t.Context(),
		f.tx,
		f.selection,
		f.started.Challenge,
		f.input,
	); err != nil {
		t.Fatal(err)
	}
	f.tx.ExpiresAt = time.Now().Add(time.Minute)
	next, err := f.method.Start(t.Context(), f.tx, f.selection, StartInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !next.Challenge.ExpiresAt.Equal(f.tx.ExpiresAt) ||
		next.Challenge.ID == f.started.Challenge.ID ||
		string(next.Challenge.State) == string(f.started.Challenge.State) {
		t.Fatal("challenge was not capped or freshly randomized")
	}
}

func TestCallbackRejectsBeforeExchange(t *testing.T) {
	f := newProviderFixture(t)
	tests := []struct {
		name   string
		change func(*entity.Transaction, *entity.MethodSelection, *entity.Challenge, *gateway.MethodInput)
	}{
		{
			"nil input",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, i *gateway.MethodInput) {
				*i = nil
			},
		},
		{
			"typed nil",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, i *gateway.MethodInput) {
				*i = (*CallbackInput)(nil)
			},
		},
		{
			"wrong type",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, i *gateway.MethodInput) {
				*i = StartInput{}
			},
		},
		{
			"wrong state",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, i *gateway.MethodInput) {
				*i = CallbackInput{
					Code:  "provider-code",
					State: strings.Repeat("x", encodedRandomLength),
				}
			},
		},
		{
			"denied",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, i *gateway.MethodInput) {
				*i = CallbackInput{Error: "access_denied", State: f.input.State}
			},
		},
		{
			"code and error",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, i *gateway.MethodInput) {
				*i = CallbackInput{
					Code:  "provider-code",
					Error: "access_denied",
					State: f.input.State,
				}
			},
		},
		{
			"oversized code",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, i *gateway.MethodInput) {
				*i = CallbackInput{Code: strings.Repeat("x", maxCodeBytes+1), State: f.input.State}
			},
		},
		{
			"missing code",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, i *gateway.MethodInput) {
				*i = CallbackInput{State: f.input.State}
			},
		},
		{
			"tenant",
			func(tx *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, _ *gateway.MethodInput) {
				tx.TenantID = "other"
			},
		},
		{
			"transaction",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *gateway.MethodInput) {
				c.TransactionID = "other"
			},
		},
		{
			"method",
			func(_ *entity.Transaction, s *entity.MethodSelection, _ *entity.Challenge, _ *gateway.MethodInput) {
				s.Method = "password"
			},
		},
		{
			"provider",
			func(_ *entity.Transaction, s *entity.MethodSelection, _ *entity.Challenge, _ *gateway.MethodInput) {
				s.Provider = "other"
			},
		},
		{
			"requirement",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *gateway.MethodInput) {
				c.Selection.Requirement = "other"
			},
		},
		{
			"missing ID",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *gateway.MethodInput) {
				c.ID = ""
			},
		},
		{
			"consumed",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *gateway.MethodInput) {
				c.Status = entity.ChallengeConsumed
			},
		},
		{
			"expired",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *gateway.MethodInput) {
				c.ExpiresAt = time.Now().Add(-time.Second)
			},
		},
		{
			"after transaction expiry",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *gateway.MethodInput) {
				c.ExpiresAt = f.tx.ExpiresAt.Add(time.Second)
			},
		},
		{
			"future creation",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *gateway.MethodInput) {
				c.CreatedAt = time.Now().Add(time.Hour)
			},
		},
		{
			"malformed state",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *gateway.MethodInput) {
				c.State = []byte("secret-invalid-json")
			},
		},
		{
			"empty private state",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *gateway.MethodInput) {
				c.State = []byte("{}")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, selection, challenge := f.tx, f.selection, *f.started.Challenge
			var input gateway.MethodInput = f.input
			tt.change(&tx, &selection, &challenge, &input)
			result, err := f.method.Callback(t.Context(), tx, selection, challenge, input)
			if !errors.Is(err, gateway.ErrProofRejected) || result.Identity != nil {
				t.Fatalf("expected proof rejection, got %v", err)
			}
		})
	}
	if _, err := f.method.Verify(
		t.Context(),
		f.tx,
		f.selection,
		nil,
		f.input,
	); !errors.Is(
		err,
		gateway.ErrProofRejected,
	) {
		t.Fatal("nil challenge accepted")
	}
	if f.posts.Load() != 0 || f.gets.Load() != 0 {
		t.Fatal("invalid callback reached provider")
	}
}

func TestIDTokenClaims(t *testing.T) {
	f := newProviderFixture(t)
	tests := []struct {
		name   string
		change func(*idClaims)
	}{
		{"issuer", func(c *idClaims) { c.Issuer = "https://attacker.example" }},
		{"audience", func(c *idClaims) { c.Audience = jwt.Audience{"other"} }},
		{"missing audience", func(c *idClaims) { c.Audience = nil }},
		{"empty subject", func(c *idClaims) { c.Subject = "" }},
		{"blank subject", func(c *idClaims) { c.Subject = " " }},
		{
			"expired",
			func(c *idClaims) { c.Expiry = jwt.NewNumericDate(time.Now().Add(-time.Second)) },
		},
		{"missing expiry", func(c *idClaims) { c.Expiry = nil }},
		{"missing issued at", func(c *idClaims) { c.IssuedAt = nil }},
		{
			"future issued at",
			func(c *idClaims) { c.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Minute)) },
		},
		{
			"future not before",
			func(c *idClaims) { c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Minute)) },
		},
		{
			"missing authorized party",
			func(c *idClaims) { c.Audience = append(c.Audience, "other") },
		},
		{"wrong authorized party", func(c *idClaims) { c.AuthorizedParty = "other" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := f.claims()
			tt.change(&claims)
			f.token = f.sign(t, claims)
			result, err := f.method.Callback(
				t.Context(),
				f.tx,
				f.selection,
				*f.started.Challenge,
				f.input,
			)
			if !errors.Is(err, gateway.ErrProofRejected) || result.Identity != nil {
				t.Fatalf("expected rejected claims, got %v", err)
			}
		})
	}
	claims := f.claims()
	claims.Audience = append(claims.Audience, "other")
	claims.AuthorizedParty = f.method.config.ClientID
	f.token = f.sign(t, claims)
	if _, err := f.method.Callback(
		t.Context(),
		f.tx,
		f.selection,
		*f.started.Challenge,
		f.input,
	); err != nil {
		t.Fatal(err)
	}
}

func TestProviderFailures(t *testing.T) {
	f := newProviderFixture(t)
	tests := []struct {
		name     string
		status   int
		body     string
		rejected bool
	}{
		{"invalid grant", 400, `{"error":"invalid_grant","error_description":"secret"}`, true},
		{"invalid client", 401, `{"error":"invalid_client"}`, false},
		{"forbidden", 403, `{"error":"secret"}`, false},
		{"rate limited", 429, `{"error":"secret"}`, false},
		{"server error", 503, `{"error":"secret"}`, false},
		{"malformed success", 200, `secret`, false},
		{"missing ID token", 200, `{"access_token":"secret"}`, false},
		{"oversized", 200, strings.Repeat("secret", maxResponseBytes), false},
		{"unexpected success status", 201, `{"id_token":"secret"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.tokenStatus, f.tokenBody = tt.status, tt.body
			before := f.posts.Load()
			_, err := f.method.Callback(
				t.Context(),
				f.tx,
				f.selection,
				*f.started.Challenge,
				f.input,
			)
			if err == nil || errors.Is(err, gateway.ErrProofRejected) != tt.rejected {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.rejected {
				var unavailable *proterror.Unavailable
				if !errors.As(err, &unavailable) {
					t.Fatalf("expected unavailable: %v", err)
				}
			}
			if strings.Contains(err.Error(), "secret") ||
				strings.Contains(err.Error(), "provider-code") {
				t.Fatal("error leaked secret")
			}
			if f.posts.Load() != before+1 {
				t.Fatal("token exchange retried")
			}
		})
	}
	if f.gets.Load() != 0 {
		t.Fatal("failed exchange requested keys")
	}
}

func TestSigningKeysAndSignature(t *testing.T) {
	f := newProviderFixture(t)
	originalToken, originalKeys := f.token, f.keys
	tests := []struct {
		name     string
		setup    func()
		rejected bool
	}{
		{"malformed token", func() { f.token = "secret" }, true},
		{"unsigned", func() { f.token = "eyJhbGciOiJub25lIn0.e30." }, true},
		{"bad signature", func() {
			parts := strings.Split(f.token, ".")
			parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"attacker"}`))
			f.token = strings.Join(parts, ".")
		}, true},
		{
			"unknown key",
			func() { f.keys = []byte(strings.ReplaceAll(string(f.keys), "telegram-key", "other-key")) },
			true,
		},
		{
			"encryption key",
			func() { f.keys = []byte(strings.ReplaceAll(string(f.keys), `"sig"`, `"enc"`)) },
			true,
		},
		{
			"wrong key algorithm",
			func() { f.keys = []byte(strings.ReplaceAll(string(f.keys), "RS256", "RS512")) },
			true,
		},
		{"malformed keys", func() { f.keyBody = "secret" }, false},
		{"empty keys", func() { f.keyBody = `{"keys":[]}` }, false},
		{"key endpoint down", func() { f.keysStatus = 503 }, false},
		{
			"oversized keys",
			func() { f.keyBody = strings.Repeat("secret", maxResponseBytes) },
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.token, f.keys, f.keyBody, f.keysStatus = originalToken, originalKeys, "", http.StatusOK
			tt.setup()
			_, err := f.method.Callback(
				t.Context(),
				f.tx,
				f.selection,
				*f.started.Challenge,
				f.input,
			)
			if err == nil || errors.Is(err, gateway.ErrProofRejected) != tt.rejected ||
				strings.Contains(err.Error(), "secret") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRedirectTimeoutAndCancellation(t *testing.T) {
	f := newProviderFixture(t)
	f.redirect = f.method.keysURL
	_, err := f.method.Callback(t.Context(), f.tx, f.selection, *f.started.Challenge, f.input)
	if err == nil || errors.Is(err, gateway.ErrProofRejected) || f.gets.Load() != 0 ||
		f.posts.Load() != 1 {
		t.Fatalf("followed redirect or misclassified: %v", err)
	}
	f.redirect = ""
	f.delay = time.Second
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err = f.method.Callback(ctx, f.tx, f.selection, *f.started.Challenge, f.input)
	if err == nil || errors.Is(err, gateway.ErrProofRejected) || f.posts.Load() != 2 {
		t.Fatalf("timeout lost fence or retried: %v", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	cancel()
	_, err = f.method.Callback(ctx, f.tx, f.selection, *f.started.Challenge, f.input)
	if !errors.Is(err, context.Canceled) || f.posts.Load() != 2 {
		t.Fatalf("canceled request dispatched: %v", err)
	}
}

func TestConcurrentCallbacks(t *testing.T) {
	f := newProviderFixture(t)
	for n := 0; n < 8; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			t.Parallel()
			if _, err := f.method.Callback(
				t.Context(),
				f.tx,
				f.selection,
				*f.started.Challenge,
				f.input,
			); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestUntrustedSigningMaterial(t *testing.T) {
	f := newProviderFixture(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		algorithm jose.SignatureAlgorithm
		key       any
		kid       string
	}{
		{"wrong signer", jose.RS256, otherKey, "telegram-key"},
		{"missing kid", jose.RS256, f.private, ""},
		{"HMAC confusion", jose.HS256, []byte(strings.Repeat("a", 32)), "telegram-key"},
		{"unconfigured algorithm", jose.ES256, ecKey, "telegram-key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer, err := jose.NewSigner(
				jose.SigningKey{Algorithm: tt.algorithm, Key: tt.key},
				(&jose.SignerOptions{}).WithHeader("kid", tt.kid).
					WithHeader("jku", "https://attacker.example/keys"),
			)
			if err != nil {
				t.Fatal(err)
			}
			f.token, err = jwt.Signed(signer).Claims(f.claims()).Serialize()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.method.Callback(
				t.Context(),
				f.tx,
				f.selection,
				*f.started.Challenge,
				f.input,
			); !errors.Is(
				err,
				gateway.ErrProofRejected,
			) {
				t.Fatalf("accepted hostile signature: %v", err)
			}
		})
	}
	f.token = f.sign(t, f.claims())
	var keys jose.JSONWebKeySet
	if err := json.Unmarshal(f.keys, &keys); err != nil {
		t.Fatal(err)
	}
	keys.Keys = append(keys.Keys, keys.Keys[0])
	f.keys, err = json.Marshal(keys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.method.Callback(
		t.Context(),
		f.tx,
		f.selection,
		*f.started.Challenge,
		f.input,
	); !errors.Is(
		err,
		gateway.ErrProofRejected,
	) {
		t.Fatal("ambiguous key ID accepted")
	}
}

func TestChallengeConfigurationBinding(t *testing.T) {
	f := newProviderFixture(t)
	for _, field := range []string{"client_id", "redirect_url", "verifier"} {
		t.Run(field, func(t *testing.T) {
			var state map[string]string
			if err := json.Unmarshal(f.started.Challenge.State, &state); err != nil {
				t.Fatal(err)
			}
			state[field] = "other-configuration"
			challenge := *f.started.Challenge
			var err error
			challenge.State, err = json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.method.Callback(
				t.Context(),
				f.tx,
				f.selection,
				challenge,
				f.input,
			); !errors.Is(
				err,
				gateway.ErrProofRejected,
			) {
				t.Fatal("configuration mismatch accepted")
			}
		})
	}
	if f.posts.Load() != 0 {
		t.Fatal("mismatched configuration reached provider")
	}
}
