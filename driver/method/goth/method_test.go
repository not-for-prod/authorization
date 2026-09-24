package goth_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"authorization/domain/entity"
	"authorization/domain/gateway"
	oauth "authorization/driver/method/goth"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func httpResponse(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func driverFixture(
	t *testing.T,
	transport http.RoundTripper,
) (*oauth.Method, entity.Transaction, entity.MethodSelection) {
	t.Helper()
	now := time.Now().UTC()
	method, err := oauth.New(
		oauth.Config{TenantID: "demo", ClientID: "google-client", ClientSecret: "client-secret",
			CallbackURL: "https://host.example/callback", HTTPClient: &http.Client{Transport: transport}},
	)
	if err != nil {
		t.Fatal(err)
	}
	tx := entity.Transaction{
		ID:        "transaction",
		TenantID:  "demo",
		Status:    entity.TransactionActive,
		CreatedAt: now,
		ExpiresAt: now.Add(time.Minute),
	}
	selection := entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}
	return method, tx, selection
}

func startProof(
	t *testing.T,
	method *oauth.Method,
	tx entity.Transaction,
	selection entity.MethodSelection,
) (gateway.StartResult, oauth.CallbackInput) {
	t.Helper()
	start, err := method.Start(t.Context(), tx, selection, oauth.StartInput{})
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := url.Parse(start.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	return start, oauth.CallbackInput{
		Code:  "authorization-code",
		State: redirect.Query().Get("state"),
	}
}

func TestGoogleFlow(t *testing.T) {
	var calls atomic.Int32
	method, tx, selection := driverFixture(
		t,
		transportFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			if r.URL.Host == "oauth2.googleapis.com" {
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if r.Form.Get("code") != "authorization-code" ||
					r.Form.Get("client_id") != "google-client" ||
					r.Form.Get("client_secret") != "client-secret" ||
					r.Form.Get("redirect_uri") != "https://host.example/callback" ||
					r.Form.Get("grant_type") != "authorization_code" {
					t.Fatalf("unexpected exchange fields")
				}
				return httpResponse(
					200,
					`{"access_token":"provider-secret","token_type":"Bearer","expires_in":3600,"id_token":"unused-token","refresh_token":"unused-refresh"}`,
				), nil
			}
			if r.URL.Host != "www.googleapis.com" || r.URL.Path != "/oauth2/v2/userinfo" ||
				r.URL.RawQuery != "" ||
				r.Header.Get("Authorization") != "Bearer provider-secret" {
				t.Fatalf("unexpected userinfo request")
			}
			return httpResponse(
				200,
				`{"id":"google-subject","email":"not-an-identity@example.com"}`,
			), nil
		}),
	)
	start, proof := startProof(t, method, tx, selection)
	redirect, _ := url.Parse(start.RedirectURL)
	if redirect.Scheme != "https" || redirect.Host != "accounts.google.com" ||
		redirect.Query().Get("client_id") != "google-client" ||
		redirect.Query().Get("access_type") != "online" ||
		!strings.Contains(redirect.Query().Get("scope"), "openid") ||
		len(proof.State) < 26 {
		t.Fatal("invalid Google redirect")
	}
	second, secondProof := startProof(t, method, tx, selection)
	if secondProof.State == proof.State || second.Challenge.ID == start.Challenge.ID {
		t.Fatal("reused correlation")
	}
	if start.Challenge.ExpiresAt.After(tx.ExpiresAt) {
		t.Fatal("challenge outlives transaction")
	}
	result, err := method.Callback(t.Context(), tx, selection, *start.Challenge, &proof)
	if err != nil {
		t.Fatal(err)
	}
	if result.UserID != nil || result.Identity == nil ||
		result.Identity.Subject != "google-subject" ||
		result.Identity.Issuer != oauth.GoogleIssuer ||
		result.Identity.Kind != "oauth" ||
		result.Evidence.Requirement != selection.Requirement ||
		result.Evidence.Provider != "google" ||
		result.Evidence.Method != "oauth" ||
		result.Evidence.VerifiedAt.IsZero() {
		t.Fatalf("wrong proof: %+v", result)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
	if strings.Contains(string(start.Challenge.State), "provider-secret") ||
		strings.Contains(string(start.Challenge.State), "authorization-code") {
		t.Fatal("persisted credentials")
	}
}

func TestInvalidProofNeverDispatches(t *testing.T) {
	method, tx, selection := driverFixture(
		t,
		transportFunc(
			func(*http.Request) (*http.Response, error) { t.Fatal("unexpected provider call"); return nil, nil },
		),
	)
	start, proof := startProof(t, method, tx, selection)
	cases := []struct {
		name   string
		mutate func(*entity.Transaction, *entity.MethodSelection, *entity.Challenge, *oauth.CallbackInput)
	}{
		{
			"state",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, p *oauth.CallbackInput) {
				p.State = "wrong"
			},
		},
		{
			"missing-code",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, p *oauth.CallbackInput) {
				p.Code = ""
			},
		},
		{
			"denied",
			func(_ *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, p *oauth.CallbackInput) {
				p.Error = "access_denied"
			},
		},
		{
			"tenant",
			func(tx *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, _ *oauth.CallbackInput) {
				tx.TenantID = "other"
			},
		},
		{
			"transaction",
			func(tx *entity.Transaction, _ *entity.MethodSelection, _ *entity.Challenge, _ *oauth.CallbackInput) {
				tx.ID = "other"
			},
		},
		{
			"requirement",
			func(_ *entity.Transaction, s *entity.MethodSelection, _ *entity.Challenge, _ *oauth.CallbackInput) {
				s.Requirement = "mfa"
			},
		},
		{
			"provider",
			func(_ *entity.Transaction, s *entity.MethodSelection, _ *entity.Challenge, _ *oauth.CallbackInput) {
				s.Provider = "github"
			},
		},
		{
			"consumed",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *oauth.CallbackInput) {
				c.Status = entity.ChallengeConsumed
			},
		},
		{
			"expired",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *oauth.CallbackInput) {
				c.ExpiresAt = time.Now().Add(-time.Second)
			},
		},
		{
			"corrupt",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *oauth.CallbackInput) {
				c.State = []byte(`{`)
			},
		},
		{
			"id",
			func(_ *entity.Transaction, _ *entity.MethodSelection, c *entity.Challenge, _ *oauth.CallbackInput) {
				c.ID = "different"
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transaction, selected, challenge, input := tx.Copy(), selection, *start.Challenge, proof
			tc.mutate(&transaction, &selected, &challenge, &input)
			if _, err := method.Callback(t.Context(), transaction, selected, challenge, input); !errors.Is(
				err,
				gateway.ErrProofRejected,
			) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	for _, input := range []gateway.MethodInput{nil, (*oauth.CallbackInput)(nil), oauth.StartInput{}} {
		if _, err := method.Callback(t.Context(), tx, selection, *start.Challenge, input); !errors.Is(
			err,
			gateway.ErrProofRejected,
		) {
			t.Fatalf("accepted input: %v", err)
		}
	}
	if _, err := method.Verify(t.Context(), tx, selection, nil, proof); !errors.Is(
		err,
		gateway.ErrProofRejected,
	) {
		t.Fatal("accepted missing challenge")
	}
	// A challenge from the right tenant but a different configured client cannot be replayed.
	other, err := oauth.New(
		oauth.Config{
			TenantID:     "demo",
			ClientID:     "other",
			ClientSecret: "secret",
			CallbackURL:  "https://host.example/callback",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.Callback(t.Context(), tx, selection, *start.Challenge, proof); !errors.Is(
		err,
		gateway.ErrProofRejected,
	) {
		t.Fatal("accepted other client")
	}
}

func TestProviderFailuresAreSanitized(t *testing.T) {
	cases := []struct {
		name      string
		tokenCode int
		tokenBody string
		userBody  string
		rejected  bool
	}{
		{
			"invalid-grant",
			400,
			`{"error":"invalid_grant","error_description":"secret-code"}`,
			"",
			true,
		},
		{"outage", 503, `secret-code`, "", false},
		{"invalid-json", 200, `secret-code`, "", false},
		{
			"invalid-id-token",
			200,
			`{"access_token":"secret-code","token_type":"Bearer","id_token":123}`,
			"",
			false,
		},
		{"oversized", 200, strings.Repeat("secret-code", 200000), "", false},
		{
			"empty-subject",
			200,
			`{"access_token":"secret-code","token_type":"Bearer"}`,
			`{"email":"secret-code"}`,
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method, tx, selection := driverFixture(
				t,
				transportFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.Host == "oauth2.googleapis.com" {
						return httpResponse(tc.tokenCode, tc.tokenBody), nil
					}
					return httpResponse(200, tc.userBody), nil
				}),
			)
			start, proof := startProof(t, method, tx, selection)
			_, err := method.Callback(t.Context(), tx, selection, *start.Challenge, proof)
			if err == nil || strings.Contains(err.Error(), "secret-code") ||
				errors.Is(err, gateway.ErrProofRejected) != tc.rejected {
				t.Fatalf("unsafe classification: %v", err)
			}
		})
	}
}

func TestCancellationReachesGoth(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	method, tx, selection := driverFixture(
		t,
		transportFunc(func(r *http.Request) (*http.Response, error) {
			cancel()
			<-r.Context().Done()
			return nil, r.Context().Err()
		}),
	)
	start, proof := startProof(t, method, tx, selection)
	_, err := method.Callback(ctx, tx, selection, *start.Challenge, proof)
	if !errors.Is(err, context.Canceled) || errors.Is(err, gateway.ErrProofRejected) {
		t.Fatalf("error = %v", err)
	}
}

func TestConfigAndStartValidation(t *testing.T) {
	for _, callback := range []string{"", ":bad", "https://", "http://remote.example/callback", "https://user:pass@host/callback", "https://host/callback#fragment"} {
		if _, err := oauth.New(oauth.Config{TenantID: "demo", ClientID: "client", ClientSecret: "secret", CallbackURL: callback}); err == nil {
			t.Fatalf("accepted %q", callback)
		}
	}
	method, tx, selection := driverFixture(t, nil)
	for _, input := range []gateway.MethodInput{nil, (*oauth.StartInput)(nil), oauth.CallbackInput{}} {
		if _, err := method.Start(t.Context(), tx, selection, input); err == nil {
			t.Fatal("accepted wrong start input")
		}
	}
	start, proof := startProof(t, method, tx, selection)
	var saved map[string]any
	if err := json.Unmarshal(start.Challenge.State, &saved); err != nil {
		t.Fatal(err)
	}
	saved["session"] = `{"AuthURL":"https://wrong.example","AccessToken":"injected"}`
	state, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	start.Challenge.State = state
	if _, err = method.Callback(t.Context(), tx, selection, *start.Challenge, proof); err == nil {
		t.Fatal("accepted injected session")
	}
}

func TestTimeoutAndRedirectAreBounded(t *testing.T) {
	for _, mode := range []string{"timeout", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			method, err := oauth.New(oauth.Config{
				TenantID: "demo", ClientID: "client", ClientSecret: "secret", CallbackURL: "https://host.example/callback",
				Timeout: 20 * time.Millisecond,
				HTTPClient: &http.Client{
					Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)
						if mode == "timeout" {
							<-r.Context().Done()
							return nil, r.Context().Err()
						}
						response := httpResponse(302, "")
						response.Header.Set("Location", "https://attacker.example/collect")
						return response, nil
					}),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			tx := entity.Transaction{
				ID:        "tx",
				TenantID:  "demo",
				Status:    entity.TransactionActive,
				ExpiresAt: now.Add(time.Minute),
			}
			selection := entity.MethodSelection{
				Requirement: "primary",
				Method:      "oauth",
				Provider:    "google",
			}
			start, proof := startProof(t, method, tx, selection)
			_, err = method.Callback(t.Context(), tx, selection, *start.Challenge, proof)
			if err == nil || errors.Is(err, gateway.ErrProofRejected) || calls.Load() != 1 {
				t.Fatalf("unbounded provider behavior: calls=%d err=%v", calls.Load(), err)
			}
			if mode == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("missing deadline: %v", err)
			}
		})
	}
}

func TestIndependentTenantClients(t *testing.T) {
	var requests atomic.Int32
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.Host == "oauth2.googleapis.com" {
			if err := r.ParseForm(); err != nil {
				return nil, err
			}
			client := r.Form.Get("client_id")
			if client != "a" && client != "b" {
				return nil, errors.New("wrong client")
			}
			if r.Form.Get("client_secret") != client+"-secret" {
				return nil, errors.New("mixed tenant credentials")
			}
			return httpResponse(200, `{"access_token":"`+client+`","token_type":"Bearer"}`), nil
		}
		return httpResponse(
			200,
			`{"id":"`+strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")+`-subject"}`,
		), nil
	})
	for _, tenant := range []entity.TenantID{"a", "b"} {
		t.Run(string(tenant), func(t *testing.T) {
			t.Parallel()
			method, err := oauth.New(
				oauth.Config{
					TenantID:     tenant,
					ClientID:     string(tenant),
					ClientSecret: string(tenant) + "-secret",
					CallbackURL:  "https://host.example/callback",
					HTTPClient:   &http.Client{Transport: transport},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			tx := entity.Transaction{
				ID:        "shared-id",
				TenantID:  tenant,
				Status:    entity.TransactionActive,
				ExpiresAt: time.Now().Add(time.Minute),
			}
			selection := entity.MethodSelection{
				Requirement: "primary",
				Method:      "oauth",
				Provider:    "google",
			}
			start, proof := startProof(t, method, tx, selection)
			result, err := method.Callback(t.Context(), tx, selection, *start.Challenge, proof)
			if err != nil {
				t.Fatal(err)
			}
			if result.Identity.Subject != string(tenant)+"-subject" {
				t.Fatal("mixed tenant result")
			}
		})
	}
}
