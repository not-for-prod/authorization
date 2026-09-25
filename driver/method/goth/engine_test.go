package goth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	auth "github.com/not-for-prod/authorization"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	events "github.com/not-for-prod/authorization/driver/event/inmem"
	identities "github.com/not-for-prod/authorization/driver/identity/inmem"
	oauth "github.com/not-for-prod/authorization/driver/method/goth"
	sessions "github.com/not-for-prod/authorization/driver/session/inmem"
	transactions "github.com/not-for-prod/authorization/driver/transaction/inmem"
	manager "github.com/not-for-prod/authorization/driver/transactionmanager/inmem"
	users "github.com/not-for-prod/authorization/driver/user/inmem"
)

type transactionContextKey struct{}
type callerContextKey struct{}
type markedManager struct{ repository.TransactionManager }

func (m markedManager) Do(ctx context.Context, operation func(context.Context) error) error {
	return m.TransactionManager.Do(ctx, func(ctx context.Context) error {
		return operation(context.WithValue(ctx, transactionContextKey{}, true))
	})
}

type engineFixture struct {
	service      auth.Authentication
	transactions *transactions.Repository
	events       *events.Publisher
	now          atomic.Int64
}

func newEngine(
	t *testing.T,
	transport http.RoundTripper,
	publisher func(repository.EventPublisher) repository.EventPublisher,
) *engineFixture {
	t.Helper()
	f := &engineFixture{}
	f.now.Store(time.Now().UTC().UnixNano())
	clock := func() time.Time { return time.Unix(0, f.now.Load()).UTC() }
	method, err := oauth.New(
		oauth.Config{
			TenantID:     "demo",
			ClientID:     "google-client",
			ClientSecret: "client-secret",
			CallbackURL:  "https://host.example/callback",
			Now:          clock,
			HTTPClient:   &http.Client{Transport: transport},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	storage := manager.New()
	f.transactions = transactions.New(storage)
	f.events = events.New(storage)
	var eventPublisher repository.EventPublisher = f.events
	if publisher != nil {
		eventPublisher = publisher(eventPublisher)
	}
	sessionManager, err := sessions.NewSessionManager("demo", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = auth.New(auth.Dependencies{
		Transactions: f.transactions, TransactionManager: markedManager{storage}, Events: eventPublisher, Users: users.New(storage), Identities: identities.New(storage),
		Access: func(ctx context.Context, _ auth.TenantID) (auth.Caller, error) {
			key := "owner"
			if other, ok := ctx.Value(callerContextKey{}).(string); ok {
				key = other
			}
			return auth.Caller{Key: key}, nil
		}, Eligible: func(context.Context, auth.TenantID, auth.User) error { return nil },
	}, auth.WithClock(clock), auth.WithLimits(auth.TransactionSettings{Lifetime: time.Minute, OperationTimeout: 5 * time.Second, MaxAttempts: 6, MaxChallenges: 4, MaxChallengeBytes: 8192, MaxPolicyNodes: 20}),
		auth.WithTenant(
			"demo",
			auth.WithSessionManager(sessionManager),
			auth.WithIntent(
				auth.IntentSignUp,
				auth.WithPolicy(
					auth.Any("primary", auth.Method(method, auth.WithProviders(oauth.Google))),
				),
				auth.WithSessionIssuance(),
			),
			auth.WithIntent(
				auth.IntentSignIn,
				auth.WithPolicy(
					auth.Any("primary", auth.Method(method, auth.WithProviders(oauth.Google))),
				),
				auth.WithSessionIssuance(),
			),
		))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *engineFixture) start(t *testing.T, intent auth.Intent) auth.CompleteCallbackRequest {
	t.Helper()
	created, err := f.service.CreateTransaction(
		t.Context(),
		auth.CreateTransactionRequest{TenantID: "demo", Intent: intent},
	)
	if err != nil {
		t.Fatal(err)
	}
	started, err := f.service.StartMethod(
		t.Context(),
		auth.StartMethodRequest{
			TenantID:      "demo",
			TransactionID: created.Transaction.ID,
			Selection: auth.MethodSelection{
				Requirement: "primary",
				Method:      "oauth",
				Provider:    "google",
			},
			Input: oauth.StartInput{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := url.Parse(started.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	return auth.CompleteCallbackRequest{
		TenantID:      "demo",
		TransactionID: created.Transaction.ID,
		AttemptID:     started.AttemptID,
		ChallengeID:   started.Challenge.ID,
		Input: oauth.CallbackInput{
			Code:  "authorization-code",
			State: redirect.Query().Get("state"),
		},
	}
}

func verifyThrough(
	ctx context.Context,
	f *engineFixture,
	entry string,
	req auth.CompleteCallbackRequest,
) (*auth.AuthenticationResult, error) {
	if entry == "callback" {
		response, err := f.service.CompleteCallback(ctx, req)
		if err != nil {
			return nil, err
		}
		return response.Result, nil
	}
	response, err := f.service.VerifyMethod(
		ctx,
		auth.VerifyMethodRequest{
			TenantID:      req.TenantID,
			TransactionID: req.TransactionID,
			AttemptID:     req.AttemptID,
			ChallengeID:   &req.ChallengeID,
			Input:         req.Input,
			DecodeInput:   req.DecodeInput,
		},
	)
	if err != nil {
		return nil, err
	}
	return response.Result, nil
}

func TestEngineGoogleSignUpSignInAndReplay(t *testing.T) {
	for _, entry := range []string{"callback", "verify"} {
		t.Run(entry, func(t *testing.T) {
			var calls atomic.Int32
			var f *engineFixture
			var request auth.CompleteCallbackRequest
			f = newEngine(t, transportFunc(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if r.Context().Value(transactionContextKey{}) != nil {
					return nil, errors.New("provider called inside transaction")
				}
				saved, err := f.transactions.Get(
					r.Context(),
					request.TenantID,
					request.TransactionID,
					"owner",
				)
				if err != nil {
					return nil, err
				}
				if saved.Claim == nil || saved.Claim.AttemptID != request.AttemptID {
					return nil, errors.New("claim not committed before dispatch")
				}
				if r.URL.Host == "oauth2.googleapis.com" {
					return httpResponse(
						200,
						`{"access_token":"provider-secret","token_type":"Bearer"}`,
					), nil
				}
				return httpResponse(
					200,
					`{"id":"google-subject","email":"unused@example.com"}`,
				), nil
			}), nil)
			var registered auth.UserID
			for _, intent := range []auth.Intent{auth.IntentSignUp, auth.IntentSignIn} {
				request = f.start(t, intent)
				wrong := request
				wrong.AttemptID = "wrong"
				if _, err := verifyThrough(t.Context(), f, entry, wrong); err == nil {
					t.Fatal("accepted wrong attempt")
				}
				if _, err := verifyThrough(context.WithValue(t.Context(), callerContextKey{}, "attacker"), f, entry, request); err == nil {
					t.Fatal("accepted wrong caller")
				}
				result, err := verifyThrough(t.Context(), f, entry, request)
				if err != nil {
					t.Fatal(err)
				}
				if result == nil || result.Session == nil || result.Principal.UserID == "" {
					t.Fatal("missing authenticated result")
				}
				if registered == "" {
					registered = result.Principal.UserID
				} else if registered != result.Principal.UserID {
					t.Fatal("Google subject resolved to different user")
				}
				before := calls.Load()
				if _, err = verifyThrough(t.Context(), f, entry, request); err == nil {
					t.Fatal("accepted consumed attempt")
				}
				if calls.Load() != before {
					t.Fatal("replayed exchange")
				}
				saved, err := f.transactions.Get(
					t.Context(),
					"demo",
					request.TransactionID,
					"owner",
				)
				if err != nil {
					t.Fatal(err)
				}
				challenge, err := saved.Challenge(request.ChallengeID)
				if err != nil {
					t.Fatal(err)
				}
				if saved.Claim != nil || saved.Pending != nil || len(saved.Evidence) != 1 ||
					challenge.Status != entity.ChallengeConsumed ||
					len(challenge.State) != 0 {
					t.Fatal("proof not atomically consumed")
				}
			}
			emitted, err := f.events.List(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(emitted)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"provider-secret", "authorization-code", "client-secret", "unused@example.com"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatal("secret leaked to events")
				}
			}
			if calls.Load() != 4 {
				t.Fatalf("provider calls = %d", calls.Load())
			}
		})
	}
}

func TestEngineRejectionAndAmbiguousRecovery(t *testing.T) {
	for _, entry := range []string{"callback", "verify"} {
		t.Run(entry, func(t *testing.T) {
			var calls atomic.Int32
			f := newEngine(t, transportFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("lost exchange response")
			}), nil)
			request := f.start(t, auth.IntentSignUp)
			invalid := request
			proof := request.Input.(oauth.CallbackInput)
			proof.State = "wrong"
			invalid.Input = proof
			if _, err := verifyThrough(t.Context(), f, entry, invalid); err == nil {
				t.Fatal("accepted wrong state")
			}
			saved, err := f.transactions.Get(t.Context(), "demo", request.TransactionID, "owner")
			if err != nil {
				t.Fatal(err)
			}
			if saved.Claim != nil || saved.Pending == nil || len(saved.Evidence) != 0 ||
				calls.Load() != 0 {
				t.Fatal("definitive rejection not persisted")
			}
			if _, err := verifyThrough(t.Context(), f, entry, request); err == nil {
				t.Fatal("accepted ambiguous exchange")
			}
			saved, err = f.transactions.Get(t.Context(), "demo", request.TransactionID, "owner")
			if err != nil {
				t.Fatal(err)
			}
			if saved.Claim == nil {
				t.Fatal("uncertain claim rolled back")
			}
			if _, err := verifyThrough(t.Context(), f, entry, request); !errors.Is(
				err,
				entity.ErrTransactionBusy,
			) {
				t.Fatalf("retry = %v", err)
			}
			f.now.Add(int64(6 * time.Second))
			if _, err := verifyThrough(t.Context(), f, entry, request); !errors.Is(
				err,
				entity.ErrTransactionAbandoned,
			) {
				t.Fatalf("expired retry = %v", err)
			}
			saved, err = f.transactions.Get(t.Context(), "demo", request.TransactionID, "owner")
			if err != nil {
				t.Fatal(err)
			}
			if saved.Status != entity.TransactionAbandoned || saved.Claim != nil ||
				saved.Pending != nil ||
				len(saved.Challenges[0].State) != 0 ||
				calls.Load() != 1 {
				t.Fatal("unsafe ambiguity recovery")
			}
			emitted, err := f.events.List(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			foundRejection, foundAbandonment := false, false
			for _, event := range emitted {
				foundRejection = foundRejection || event.Type == "MethodProofRejected"
				foundAbandonment = foundAbandonment || event.Type == "TransactionAbandoned"
			}
			if !foundRejection || !foundAbandonment {
				t.Fatal("missing recovery events")
			}
		})
	}
}

func TestConcurrentCallbackAndVerifyExchangeOnce(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var exchanges atomic.Int32
	f := newEngine(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Context().Value(transactionContextKey{}) != nil {
			return nil, errors.New("network call inside transaction")
		}
		if r.URL.Host == "oauth2.googleapis.com" {
			if exchanges.Add(1) != 1 {
				return nil, errors.New("duplicate exchange")
			}
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return httpResponse(200, `{"access_token":"token","token_type":"Bearer"}`), nil
		}
		return httpResponse(200, `{"id":"subject"}`), nil
	}), nil)
	request := f.start(t, auth.IntentSignUp)
	result := make(chan error, 1)
	go func() { _, err := verifyThrough(t.Context(), f, "callback", request); result <- err }()
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("first call failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("provider not reached")
	}
	_, err := verifyThrough(t.Context(), f, "verify", request)
	close(release)
	if !errors.Is(err, entity.ErrTransactionBusy) {
		t.Fatalf("concurrent request = %v", err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if exchanges.Load() != 1 {
		t.Fatal("duplicate exchange")
	}
}

type failingPublisher struct {
	repository.EventPublisher
	failOn string
}

func (p failingPublisher) Publish(ctx context.Context, events []entity.TransactionEvent) error {
	for _, event := range events {
		if event.Type == p.failOn {
			return errors.New("outbox unavailable")
		}
	}
	return p.EventPublisher.Publish(ctx, events)
}

func TestClaimAndAcceptancePublicationAreAtomic(t *testing.T) {
	for _, failOn := range []string{"MethodVerificationRequested", "EvidenceAccepted"} {
		t.Run(failOn, func(t *testing.T) {
			var calls atomic.Int32
			f := newEngine(t, transportFunc(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if r.URL.Host == "oauth2.googleapis.com" {
					return httpResponse(200, `{"access_token":"token","token_type":"Bearer"}`), nil
				}
				return httpResponse(200, `{"id":"subject"}`), nil
			}), func(p repository.EventPublisher) repository.EventPublisher { return failingPublisher{p, failOn} })
			request := f.start(t, auth.IntentSignUp)
			if _, err := verifyThrough(t.Context(), f, "callback", request); err == nil {
				t.Fatal("accepted failed publication")
			}
			saved, err := f.transactions.Get(t.Context(), "demo", request.TransactionID, "owner")
			if err != nil {
				t.Fatal(err)
			}
			if len(saved.Evidence) != 0 || saved.Pending == nil ||
				saved.Challenges[0].Status != entity.ChallengePending {
				t.Fatal("partial evidence commit")
			}
			if failOn == "MethodVerificationRequested" {
				if saved.Claim != nil || calls.Load() != 0 {
					t.Fatal("dispatch before claim committed")
				}
			} else {
				if saved.Claim == nil || calls.Load() != 2 {
					t.Fatal("lost committed dispatch claim")
				}
				if _, err := verifyThrough(t.Context(), f, "callback", request); !errors.Is(err, entity.ErrTransactionBusy) {
					t.Fatalf("retry = %v", err)
				}
				if calls.Load() != 2 {
					t.Fatal("re-exchanged after persistence failure")
				}
			}
		})
	}
}

func TestAttemptBudgetSurvivesRejection(t *testing.T) {
	f := newEngine(
		t,
		transportFunc(
			func(*http.Request) (*http.Response, error) { return nil, errors.New("unexpected network call") },
		),
		nil,
	)
	request := f.start(t, auth.IntentSignUp)
	proof := request.Input.(oauth.CallbackInput)
	proof.State = "invalid"
	request.Input = proof
	// Start consumes one of the six configured dispatch attempts.
	for range 5 {
		if _, err := verifyThrough(t.Context(), f, "callback", request); err == nil {
			t.Fatal("accepted bad proof")
		}
	}
	if _, err := verifyThrough(t.Context(), f, "callback", request); !errors.Is(
		err,
		entity.ErrAttemptLimit,
	) {
		t.Fatalf("attempt budget reset: %v", err)
	}
}

func TestLateProviderResultCannotCommit(t *testing.T) {
	var f *engineFixture
	f = newEngine(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "oauth2.googleapis.com" {
			return httpResponse(200, `{"access_token":"token","token_type":"Bearer"}`), nil
		}
		f.now.Add(int64(6 * time.Second))
		return httpResponse(200, `{"id":"subject"}`), nil
	}), nil)
	request := f.start(t, auth.IntentSignUp)
	if _, err := verifyThrough(t.Context(), f, "callback", request); !errors.Is(
		err,
		entity.ErrTransactionAbandoned,
	) {
		t.Fatalf("late result = %v", err)
	}
	saved, err := f.transactions.Get(t.Context(), "demo", request.TransactionID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Evidence) != 0 || saved.Claim == nil {
		t.Fatal("late proof accepted")
	}
	if _, err := verifyThrough(t.Context(), f, "callback", request); !errors.Is(
		err,
		entity.ErrTransactionAbandoned,
	) {
		t.Fatalf("recovery = %v", err)
	}
}
