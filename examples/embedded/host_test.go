package embedded_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	auth "authorization"
	sessioninmem "authorization/driver/session/inmem"
	managerinmem "authorization/driver/transactionmanager/inmem"
	userinmem "authorization/driver/user/inmem"

	"google.golang.org/protobuf/types/known/structpb"
)

// These adapters are deliberately single-threaded test doubles, not deployment
// adapters. Embedding ports makes unexpected calls to unused methods fail loudly.
type transactionStore struct {
	auth.TransactionRepository
	rows   map[auth.TransactionID]auth.Transaction
	events []auth.TransactionEvent
}

func (s *transactionStore) Create(_ context.Context, tx auth.Transaction) error {
	if _, exists := s.rows[tx.ID]; exists {
		return auth.ErrAlreadyExists
	}

	s.rows[tx.ID] = tx.Copy()

	return nil
}

func (s *transactionStore) Get(_ context.Context, tenant auth.TenantID, id auth.TransactionID, ownerKey string, _ ...auth.SelectOption) (*auth.Transaction, error) {
	tx, ok := s.rows[id]
	if !ok || tx.TenantID != tenant || tx.OwnerKey != ownerKey {
		return nil, auth.ErrNotFound
	}

	copy := tx.Copy()

	return &copy, nil
}

func (s *transactionStore) Update(_ context.Context, tx auth.Transaction) error {
	if _, exists := s.rows[tx.ID]; !exists {
		return auth.ErrNotFound
	}

	s.rows[tx.ID] = tx.Copy()

	return nil
}

func (s *transactionStore) Do(ctx context.Context, operation func(context.Context) error) error {
	beforeEvents := len(s.events)

	before := make(map[auth.TransactionID]auth.Transaction, len(s.rows))
	for id, tx := range s.rows {
		before[id] = tx.Copy()
	}

	if err := operation(ctx); err != nil {
		s.rows = before
		s.events = s.events[:beforeEvents]

		return err
	}

	return nil
}

type users struct{ auth.UserRepository }

func (users) Get(_ context.Context, id auth.UserID, _ ...auth.SelectOption) (*auth.User, error) {
	if id != "alice" {
		return nil, auth.ErrNotFound
	}

	return &auth.User{ID: id, Status: "active"}, nil
}

type passwordInput struct{ Password string }

func (passwordInput) AuthenticationInput() {}

type passwordMethod struct{}

func (passwordMethod) Name() auth.MethodID { return "password" }

func (passwordMethod) Available(context.Context, auth.Transaction, auth.MethodSelection) (bool, error) {
	return true, nil
}

func (passwordMethod) Start(context.Context, auth.Transaction, auth.MethodSelection, auth.MethodInput) (auth.StartResult, error) {
	return auth.StartResult{}, nil
}

func (passwordMethod) Verify(_ context.Context, _ auth.Transaction, selection auth.MethodSelection, _ *auth.Challenge, input auth.MethodInput) (auth.VerificationResult, error) {
	value, ok := input.(passwordInput)
	if !ok || value.Password != "test-only-password" {
		return auth.VerificationResult{}, auth.ErrProofRejected
	}

	id := auth.UserID("alice")

	return auth.VerificationResult{
		UserID:   &id,
		Evidence: auth.Evidence{Requirement: selection.Requirement, Method: selection.Method, VerifiedAt: time.Now()},
	}, nil
}

type callerKey struct{}

func dependencies() auth.Dependencies {
	store := &transactionStore{rows: make(map[auth.TransactionID]auth.Transaction)}

	return auth.Dependencies{
		Events: store, Transactions: store,
		TransactionManager: store,
		Users:              users{},
		Identities:         struct{ auth.IdentityRepository }{}, // Direct-user driver never resolves identities.
		Access: func(ctx context.Context, tenant auth.TenantID) (auth.Caller, error) {
			caller, ok := ctx.Value(callerKey{}).(auth.Caller)
			if !ok || tenant != "demo" {
				return auth.Caller{}, errors.New("unauthorized caller")
			}

			return caller, nil
		},
		Eligible: func(_ context.Context, _ auth.TenantID, user auth.User) error {
			if user.Status != "active" {
				return errors.New("inactive user")
			}

			return nil
		},
	}
}

// The policy selects and registers its exact driver in one place.
func options() []auth.Option {
	return []auth.Option{
		auth.WithLimits(auth.TransactionSettings{
			Lifetime: 5 * time.Minute, OperationTimeout: 15 * time.Second,
			MaxAttempts: 10, MaxChallenges: 4, MaxChallengeBytes: 16 * 1024, MaxPolicyNodes: 64,
		}),
		auth.WithTenant("demo",
			auth.WithIntent("sign_in",
				auth.WithPolicy(auth.Any("primary", auth.Method(passwordMethod{}))),
			),
		),
	}
}

func TestEmbeddedAuthentication(t *testing.T) {
	service, err := auth.New(dependencies(), options()...)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.WithValue(t.Context(), callerKey{}, auth.Caller{Key: "trusted-test-caller"})
	created, err := service.CreateTransaction(ctx, auth.CreateTransactionRequest{TenantID: "demo", Intent: "sign_in"})
	if err != nil {
		t.Fatal(err)
	}

	if len(created.Transaction.AvailableMethods) != 1 {
		t.Fatalf("unexpected methods: %+v", created.Transaction.AvailableMethods)
	}

	req := auth.VerifyMethodRequest{TenantID: "demo", TransactionID: created.Transaction.ID,
		Input: passwordInput{Password: "wrong"}}
	started, err := service.StartMethod(ctx, auth.StartMethodRequest{TenantID: req.TenantID, TransactionID: req.TransactionID, Selection: created.Transaction.AvailableMethods[0], Input: passwordInput{}})
	if err != nil {
		t.Fatal(err)
	}
	req.AttemptID = started.AttemptID
	if _, err := service.VerifyMethod(ctx, req); !errors.Is(err, auth.ErrProofRejected) {
		t.Fatalf("expected proof rejection, got %v", err)
	}

	req.Input = passwordInput{Password: "test-only-password"}
	verified, err := service.VerifyMethod(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	if verified.Result == nil || verified.Result.Principal.UserID != "alice" || !verified.Transaction.PolicySatisfied {
		t.Fatalf("unexpected result: %+v", verified)
	}

	progress, err := service.GetTransaction(ctx, auth.GetTransactionRequest{TenantID: "demo", TransactionID: created.Transaction.ID})
	if err != nil || !progress.Transaction.PolicySatisfied {
		t.Fatalf("completed progress: %+v, error: %v", progress, err)
	}
}

func TestHTTPWiring(t *testing.T) {
	service, err := auth.New(dependencies(), options()...)
	if err != nil {
		t.Fatal(err)
	}

	handler, err := auth.NewHTTPHandler(t.Context(), service,
		func(_ context.Context, _ auth.TenantID, _ auth.MethodSelection, _ auth.OperationKind, input *structpb.Struct) (auth.MethodInput, error) {
			return passwordInput{Password: input.GetFields()["password"].GetStringValue()}, nil
		})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/tenants/demo/auth/transactions", strings.NewReader(`{"intent":"INTENT_SIGN_IN"}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), callerKey{}, auth.Caller{Key: "trusted-test-caller"}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("HTTP %d: %s", response.Code, response.Body.String())
	}
}

func TestInvalidConfig(t *testing.T) {
	service, err := auth.New(auth.Dependencies{})
	if err == nil || service != nil {
		t.Fatalf("expected a nil service and configuration error, got %v, %v", service, err)
	}
}

// These imports are also compiled from an unrelated module by TestExternalHost.
func TestPublicInMemoryDrivers(t *testing.T) {
	store := managerinmem.New()
	users := userinmem.New(store)

	var _ auth.TransactionManager = store
	if err := store.Do(t.Context(), func(ctx context.Context) error {
		return users.Create(ctx, auth.User{ID: "alice"})
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := users.Get(t.Context(), "alice"); err != nil {
		t.Fatal(err)
	}

	sessions, err := sessioninmem.NewSessionManager("main", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var _ auth.SessionManager = sessions
}

func (s *transactionStore) Publish(_ context.Context, events []auth.TransactionEvent) error {
	for _, event := range events {
		s.events = append(s.events, event.Copy())
	}

	return nil
}
