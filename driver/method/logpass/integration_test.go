package logpass_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	auth "authorization"
	eventinmem "authorization/driver/event/inmem"
	identityinmem "authorization/driver/identity/inmem"
	"authorization/driver/method/logpass"
	sessioninmem "authorization/driver/session/inmem"
	transactioninmem "authorization/driver/transaction/inmem"
	managerinmem "authorization/driver/transactionmanager/inmem"
	userinmem "authorization/driver/user/inmem"
)

func TestEngineSignIn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	manager := managerinmem.New()
	identities, users := identityinmem.New(manager), userinmem.New(manager)
	events := eventinmem.New(manager)
	if err := manager.Do(ctx, func(ctx context.Context) error {
		if err := users.Create(ctx, auth.User{ID: "alice-id", Status: auth.UserStatusActive}); err != nil {
			return err
		}
		return identities.Create(ctx, auth.Identity{
			ID: "local-alice", UserID: "alice-id", Issuer: "demo/local", Subject: "alice", PasswordHash: "encoded-test-hash",
		})
	}); err != nil {
		t.Fatal(err)
	}
	// This deterministic test callback exercises the boundary, not a password algorithm.
	driver, err := logpass.New(identities, logpass.Config{
		Issuer: "demo/local", DummyHash: "encoded-dummy-hash",
		VerifyPassword: func(_ context.Context, hash, password string) (bool, error) {
			return hash == "encoded-test-hash" && password == "test-password-secret", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := sessioninmem.NewSessionManager("demo", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := auth.New(auth.Dependencies{
		TransactionManager: manager, Transactions: transactioninmem.New(manager),
		Identities: identities, Users: users, Events: events,
		Access: func(context.Context, auth.TenantID) (auth.Caller, error) {
			return auth.Caller{Key: "trusted-browser"}, nil
		},
		Eligible: func(_ context.Context, _ auth.TenantID, user auth.User) error {
			if user.Status != auth.UserStatusActive {
				return errors.New("inactive user")
			}
			return nil
		},
	}, auth.WithLimits(auth.TransactionSettings{
		Lifetime: time.Minute, OperationTimeout: time.Second, MaxAttempts: 4,
		MaxChallenges: 1, MaxChallengeBytes: 1024, MaxPolicyNodes: 10,
	}), auth.WithTenant(
		"demo",
		auth.WithSessionManager(sessions),
		auth.WithIntent(
			"sign_in",
			auth.WithSessionIssuance(),
			auth.WithPolicy(auth.Any("primary", auth.Method(driver))),
		),
	))
	if err != nil {
		t.Fatal(err)
	}
	created, err := engine.CreateTransaction(
		ctx,
		auth.CreateTransactionRequest{TenantID: "demo", Intent: "sign_in"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Transaction.AvailableMethods) != 1 {
		t.Fatal("password method not available")
	}
	started, err := engine.StartMethod(ctx, auth.StartMethodRequest{
		TenantID: "demo", TransactionID: created.Transaction.ID,
		Selection: created.Transaction.AvailableMethods[0], Input: logpass.StartInput{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.AttemptID == "" || started.Challenge != nil {
		t.Fatal("invalid direct attempt")
	}
	request := auth.VerifyMethodRequest{
		TenantID:      "demo",
		TransactionID: created.Transaction.ID,
		AttemptID:     started.AttemptID,
		Input:         logpass.VerifyInput{Login: "alice", Password: "wrong-password"},
	}
	if _, err := engine.VerifyMethod(ctx, request); !errors.Is(err, auth.ErrProofRejected) {
		t.Fatalf("wrong password: %v", err)
	}
	request.Input = logpass.VerifyInput{Login: "alice", Password: "test-password-secret"}
	verified, err := engine.VerifyMethod(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Result == nil || verified.Result.Principal.UserID != "alice-id" ||
		verified.Result.Session == nil ||
		verified.Result.Session.Token == "" {
		t.Fatal("password proof did not issue a session for the canonical user")
	}
	storedEvents, err := events.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{verified, storedEvents} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "test-password-secret") ||
			strings.Contains(string(data), "encoded-test-hash") {
			t.Fatal("credential leaked into result/events")
		}
	}
}
