package authenticationservice_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	authenticationservice "authorization/application/service/authentication-service"
	"authorization/domain/application"
	"authorization/domain/entity"
	"authorization/domain/gateway"
	"authorization/domain/repository"
	eventinmem "authorization/driver/event/inmem"
	identityinmem "authorization/driver/identity/inmem"
	transactioninmem "authorization/driver/transaction/inmem"
	managerinmem "authorization/driver/transactionmanager/inmem"
	userinmem "authorization/driver/user/inmem"
	"authorization/internal/testutil/authfixture"
)

// The fixture's password-labelled mock can return a verified identity. It is a
// direct-proof test double, not a password enrollment implementation.
func signUpFixture(t *testing.T, secondProof bool) (*authfixture.Fixture, *managerinmem.Manager) {
	t.Helper()
	f := authfixture.New()
	manager := managerinmem.New()
	f.Config.Transactions = transactioninmem.New(manager)
	f.Config.Users = userinmem.New(manager)
	f.Config.Identities = identityinmem.New(manager)
	f.Config.TransactionManager = manager
	f.Config.Events = eventinmem.New(manager)
	f.Policy.Intent = entity.IntentSignUp
	primary := entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{entity.MethodRequirement{Method: "password"}, entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google"}}}}
	f.Policy.Root = primary
	if secondProof {
		f.Policy.Root = entity.AllOfRequirement{Children: []entity.Requirement{primary, entity.AnyOfRequirement{ID: "confirmation", Children: []entity.Requirement{entity.MethodRequirement{Method: "password"}}}}}
	}
	tenant := f.Config.Tenants["demo"]
	tenant.Intents = map[entity.Intent]authenticationservice.IntentConfig{entity.IntentSignUp: {IssueSession: true}}
	f.Config.Tenants["demo"] = tenant
	return f, manager
}

func signUpAttempt(t *testing.T, svc application.Authentication, ctx context.Context, id entity.TransactionID, requirement, subject string) application.VerifyMethodRequest {
	t.Helper()
	req := startVerification(t, svc, ctx, id, requirement, "password", subject)
	req.Input = authfixture.Input{UserID: entity.UserID(subject), Identity: true}
	return req
}

func TestSignUpCollectsProofBeforeCreationAndReplaysCompletion(t *testing.T) {
	f, manager := signUpFixture(t, true)
	svc, err := authenticationservice.NewImplementation(f.Config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authfixture.WithCaller(t.Context(), "owner", nil)
	created, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: entity.IntentSignUp})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Transaction.ID
	if _, err := svc.VerifyMethod(ctx, signUpAttempt(t, svc, ctx, id, "primary", "new-user")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Config.Identities.GetBySubject(ctx, "google", "new-user"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("identity created before policy satisfaction", err)
	}
	stored, err := f.Config.Transactions.Get(ctx, "demo", id, "owner")
	if err != nil || stored.UserID != nil || stored.VerifiedIdentity == nil {
		t.Fatal("incorrect pending registration", stored, err)
	}
	copied := stored.Copy()
	copied.VerifiedIdentity.Subject = "mutated"
	if stored.VerifiedIdentity.Subject != "new-user" {
		t.Fatal("pending identity alias")
	}
	// A different identity cannot complete the first subject's registration.
	mismatch := signUpAttempt(t, svc, ctx, id, "confirmation", "someone-else")
	if _, err := svc.VerifyMethod(ctx, mismatch); !errors.Is(err, entity.ErrSubjectMismatch) {
		t.Fatal("subject change accepted", err)
	}
	req := signUpAttempt(t, svc, ctx, id, "confirmation", "new-user")
	result, err := svc.VerifyMethod(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := f.Config.Identities.GetBySubject(ctx, "google", "new-user")
	if err != nil {
		t.Fatal(err)
	}
	user, err := f.Config.Users.Get(ctx, identity.UserID)
	if err != nil || user.Status != entity.UserStatusActive || result.Result.Principal.UserID != user.ID {
		t.Fatal("wrong registered user", result, err)
	}
	replay, err := svc.VerifyMethod(ctx, req)
	if err != nil || replay.Result.Session.Token != result.Result.Session.Token || f.SessionCount() != 1 {
		t.Fatal("completion was not idempotent", err)
	}
	data, release, err := manager.Access(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if len(data.Users) != 1 || len(data.Identities) != 1 {
		t.Fatal("duplicate accounts")
	}
}

func TestSignUpConcurrentIdentityClaim(t *testing.T) {
	f, manager := signUpFixture(t, false)
	svc, err := authenticationservice.NewImplementation(f.Config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authfixture.WithCaller(t.Context(), "owner", nil)
	requests := make([]application.VerifyMethodRequest, 2)
	for j := range requests {
		created, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: entity.IntentSignUp})
		if err != nil {
			t.Fatal(err)
		}
		requests[j] = signUpAttempt(t, svc, ctx, created.Transaction.ID, "primary", "same-subject")
	}
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, req := range requests {
		group.Go(func() { _, err := svc.VerifyMethod(ctx, req); errs <- err })
	}
	group.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, repository.ErrAlreadyExists) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 || f.SessionCount() != 1 {
		t.Fatal("unexpected race outcome", success, conflict)
	}
	data, release, err := manager.Access(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if len(data.Users) != 1 || len(data.Identities) != 1 {
		t.Fatal("orphan or duplicate account")
	}
}

type rejectRegistrationEvent struct{ repository.EventPublisher }

func (p rejectRegistrationEvent) Publish(ctx context.Context, events []entity.TransactionEvent) error {
	for _, event := range events {
		if event.Type == "UserRegistered" {
			return errors.New("injected registration write failure")
		}
	}
	return p.EventPublisher.Publish(ctx, events)
}

func TestSignUpRollbackAndRejectedProof(t *testing.T) {
	for _, scenario := range []string{"publication failure", "user ID proof", "invalid proof", "ineligible user"} {
		t.Run(scenario, func(t *testing.T) {
			f, manager := signUpFixture(t, false)
			if scenario == "publication failure" {
				f.Config.Events = rejectRegistrationEvent{f.Config.Events}
			}
			if scenario == "ineligible user" {
				f.Config.Eligible = func(context.Context, entity.TenantID, entity.User) error { return errors.New("not eligible") }
			}
			svc, err := authenticationservice.NewImplementation(f.Config)
			if err != nil {
				t.Fatal(err)
			}
			ctx := authfixture.WithCaller(t.Context(), "owner", nil)
			created, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: entity.IntentSignUp})
			if err != nil {
				t.Fatal(err)
			}
			req := signUpAttempt(t, svc, ctx, created.Transaction.ID, "primary", "new-user")
			if scenario == "user ID proof" {
				req.Input = authfixture.Input{UserID: "alice"}
			}
			if scenario == "invalid proof" {
				req.Input = authfixture.Input{Reject: true}
			}
			if _, err := svc.VerifyMethod(ctx, req); err == nil {
				t.Fatal("registration unexpectedly succeeded")
			}
			data, release, err := manager.Access(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if len(data.Users) != 0 || len(data.Identities) != 0 || f.SessionCount() != 0 {
				t.Fatal("failed registration leaked account state")
			}
		})
	}
}

func TestSignUpCallbackAndSessionRecovery(t *testing.T) {
	f, _ := signUpFixture(t, false)
	issue := f.Sessions.CreateFunc
	first := true
	f.Sessions.CreateFunc = func(ctx context.Context, req gateway.CreateSessionRequest) (entity.IssuedSession, error) {
		if first {
			first = false
			return entity.IssuedSession{}, errors.New("session service unavailable")
		}
		return issue(ctx, req)
	}
	svc, err := authenticationservice.NewImplementation(f.Config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authfixture.WithCaller(t.Context(), "owner", nil)
	created, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: entity.IntentSignUp})
	if err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartMethod(ctx, application.StartMethodRequest{TenantID: "demo", TransactionID: created.Transaction.ID, Selection: entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}, Input: authfixture.Input{}})
	if err != nil {
		t.Fatal(err)
	}
	req := application.CompleteCallbackRequest{TenantID: "demo", TransactionID: created.Transaction.ID, AttemptID: started.AttemptID, ChallengeID: started.Challenge.ID, Input: authfixture.Input{UserID: "new-user", Identity: true}}
	if _, err := svc.CompleteCallback(ctx, req); err == nil {
		t.Fatal("expected session failure")
	}
	identity, err := f.Config.Identities.GetBySubject(ctx, "google", "new-user")
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.CompleteCallback(ctx, req)
	if err != nil || result.Result.Principal.UserID != identity.UserID || f.SessionCount() != 1 {
		t.Fatal("registration recovery failed", err)
	}
	if len(f.Methods["oauth"].CallbackCalls()) != 1 {
		t.Fatal("replayed proof after completed registration")
	}
}
