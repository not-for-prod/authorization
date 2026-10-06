package authenticationservice_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/internal/testutil/authfixture"
)

func TestPendingAttemptGuardsAndReplay(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)
	req := verify(id, "alice")
	req.AttemptID = "not-started"
	before := f.Store.Events()
	if _, err := svc.VerifyMethod(ctx, req); !errors.Is(err, entity.ErrInvalidTransition) {
		t.Fatalf("verify before start: %v", err)
	}
	if len(f.Methods["password"].VerifyCalls()) != 0 || !reflect.DeepEqual(before, f.Store.Events()) {
		t.Fatal("invalid verification dispatched or emitted events")
	}
	start := application.StartMethodRequest{TenantID: "demo", TransactionID: id, Selection: entity.MethodSelection{Requirement: "primary", Method: "password"}, Input: authfixture.Input{}}
	started, err := svc.StartMethod(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	if started.AttemptID == "" || started.Challenge != nil || started.Transaction.PendingAttempt == nil || !started.Transaction.PendingAttempt.Ready {
		t.Fatalf("missing direct attempt: %+v", started)
	}
	before = f.Store.Events()
	replay, err := svc.StartMethod(ctx, start)
	if err != nil || replay.AttemptID != started.AttemptID || len(f.Methods["password"].StartCalls()) != 1 || !reflect.DeepEqual(before, f.Store.Events()) {
		t.Fatalf("start replay: %+v %v", replay, err)
	}
	other := start
	other.Selection = entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}
	if _, err := svc.StartMethod(ctx, other); !errors.Is(err, entity.ErrInvalidTransition) {
		t.Fatalf("second pending start: %v", err)
	}
	if _, err := svc.VerifyMethod(ctx, req); !errors.Is(err, entity.ErrInvalidTransition) {
		t.Fatalf("wrong attempt: %v", err)
	}
	req.AttemptID = started.AttemptID
	if _, err := svc.VerifyMethod(ctx, req); err != nil {
		t.Fatal(err)
	}
	before = f.Store.Events()
	if _, err := svc.VerifyMethod(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(f.Methods["password"].VerifyCalls()) != 1 || !reflect.DeepEqual(before, f.Store.Events()) {
		t.Fatal("verification replay repeated transition")
	}
	mfa := startVerification(t, svc, ctx, id, "mfa", "totp", "alice")
	result, err := svc.VerifyMethod(ctx, mfa)
	if err != nil || result.Result == nil || result.Result.Session == nil {
		t.Fatalf("synchronous finalization: %+v %v", result, err)
	}
	before = f.Store.Events()
	if _, err := svc.VerifyMethod(ctx, mfa); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, f.Store.Events()) || f.SessionCount() != 1 {
		t.Fatal("completion retry emitted event or duplicated session")
	}
	events := f.Store.Events()
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	event := events[0]
	if event.Type != "AuthenticationCompleted" || event.After.Intent != entity.IntentSignIn ||
		event.Before == nil || event.Before.Status != entity.TransactionFinalizing ||
		event.After.Status != entity.TransactionCompleted || event.After.UserID != "alice" ||
		event.AggregateVersion != event.Before.Version+1 || event.After.Version != event.AggregateVersion {
		t.Fatalf("completion event: %+v", event)
	}
}

func TestRetiredAttemptCannotVerifyNewStart(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)
	old := startVerification(t, svc, ctx, id, "primary", "password", "missing")
	if _, err := svc.VerifyMethod(ctx, old); err == nil {
		t.Fatal("missing subject accepted")
	}
	next := startVerification(t, svc, ctx, id, "primary", "password", "alice")
	if next.AttemptID == old.AttemptID {
		t.Fatal("attempt reused")
	}
	old.Input = authfixture.Input{UserID: "alice"}
	calls := len(f.Methods["password"].VerifyCalls())
	if _, err := svc.VerifyMethod(ctx, old); !errors.Is(err, entity.ErrInvalidTransition) {
		t.Fatalf("stale attempt: %v", err)
	}
	if len(f.Methods["password"].VerifyCalls()) != calls {
		t.Fatal("stale request dispatched")
	}
	if _, err := svc.VerifyMethod(ctx, next); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationFailureRollsBackEvidence(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)
	req := startVerification(t, svc, ctx, id, "primary", "password", "alice")
	original := f.Methods["password"].VerifyFunc
	failure := errors.New("outbox unavailable")
	f.Methods["password"].VerifyFunc = func(ctx context.Context, tx entity.Transaction, s entity.MethodSelection, c *entity.Challenge, input gateway.MethodInput) (gateway.VerificationResult, error) {
		proof, err := original(ctx, tx, s, c, input)
		f.Store.PublishError = failure
		return proof, err
	}
	if _, err := svc.VerifyMethod(ctx, req); !errors.Is(err, failure) {
		t.Fatalf("publication failure: %v", err)
	}
	saved, err := f.Store.Get(ctx, "demo", id, "browser")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Pending == nil || saved.Claim == nil || saved.UserID != nil || len(saved.Evidence) != 0 {
		t.Fatal("evidence committed without event")
	}
	events := f.Store.Events()
	if len(events) != 0 {
		t.Fatal("rolled back event published")
	}
}

func TestPublicationFailurePreventsDispatch(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)
	before := f.Store.Events()
	failure := errors.New("outbox unavailable")
	f.Store.PublishError = failure
	if _, err := svc.StartMethod(ctx, application.StartMethodRequest{TenantID: "demo", TransactionID: id, Selection: entity.MethodSelection{Requirement: "primary", Method: "password"}, Input: authfixture.Input{}}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	saved, err := f.Store.Get(ctx, "demo", id, "browser")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Pending != nil || saved.Claim != nil || !reflect.DeepEqual(before, f.Store.Events()) || len(f.Methods["password"].StartCalls()) != 0 {
		t.Fatal("dispatch claim escaped failed publication")
	}
}

func TestConcurrentStartDispatchesOnce(t *testing.T) {
	f := authfixture.New()
	entered := make(chan struct{})
	release := make(chan struct{})
	original := f.Methods["password"].StartFunc
	f.Methods["password"].StartFunc = func(ctx context.Context, tx entity.Transaction, s entity.MethodSelection, input gateway.MethodInput) (gateway.StartResult, error) {
		close(entered)
		<-release
		return original(ctx, tx, s, input)
	}
	svc, ctx, id := app(t, f)
	req := application.StartMethodRequest{TenantID: "demo", TransactionID: id, Selection: entity.MethodSelection{Requirement: "primary", Method: "password"}, Input: authfixture.Input{}}
	done := make(chan error, 1)
	go func() { _, err := svc.StartMethod(ctx, req); done <- err }()
	<-entered
	_, err := svc.StartMethod(ctx, req)
	close(release)
	if !errors.Is(err, entity.ErrTransactionBusy) {
		t.Fatalf("competing start: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(f.Methods["password"].StartCalls()) != 1 {
		t.Fatal("multiple start dispatchers")
	}
}

func TestStartReplayPreservesChallengeAndRedirect(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)
	req := application.StartMethodRequest{TenantID: "demo", TransactionID: id,
		Selection: entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}, Input: authfixture.Input{}}
	started, err := svc.StartMethod(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if started.Challenge == nil || started.RedirectURL == "" {
		t.Fatalf("missing challenge or redirect: %+v", started)
	}
	before := f.Store.Events()
	replay, err := svc.StartMethod(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(started, replay) {
		t.Fatalf("start replay changed response: first=%+v replay=%+v", started, replay)
	}
	if len(f.Methods["oauth"].StartCalls()) != 1 || !reflect.DeepEqual(before, f.Store.Events()) {
		t.Fatal("start replay dispatched or emitted events")
	}
}
