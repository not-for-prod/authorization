package authenticationservice_test

import (
	"testing"

	authenticationservice "github.com/not-for-prod/authorization/application/service/authentication-service"
	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/internal/testutil/authfixture"
)

func TestSimpleStartVerify(t *testing.T) {
	f, _ := signUpFixture(t, true)
	svc, err := authenticationservice.NewImplementation(f.Config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authfixture.WithCaller(t.Context(), "owner", nil)
	created, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: entity.IntentSignUp})
	if err != nil {
		t.Fatal(err)
	}
	first := signUpAttempt(t, svc, ctx, created.Transaction.ID, "primary", "new-user")
	rejected := first
	rejected.Input = authfixture.Input{Reject: true}
	if _, err := svc.VerifyMethod(ctx, rejected); err == nil {
		t.Fatal("rejected proof accepted")
	}
	saved, err := f.Config.Transactions.Get(ctx, "demo", created.Transaction.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Evidence) != 0 || saved.Pending == nil || saved.Claim != nil {
		t.Fatal("failed verification changed transaction")
	}
	partial, err := svc.VerifyMethod(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Result != nil || partial.Transaction.PolicySatisfied || len(partial.Transaction.AvailableMethods) == 0 {
		t.Fatalf("bad partial progress: %+v", partial)
	}
	identity, err := f.Config.Identities.GetBySubject(ctx, "google", "new-user")
	if err == nil || identity != nil {
		t.Fatal("identity created before full policy")
	}
	second := signUpAttempt(t, svc, ctx, created.Transaction.ID, "confirmation", "new-user")
	completed, err := svc.VerifyMethod(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Result == nil || completed.Result.Session == nil || completed.Transaction.Status != entity.TransactionCompleted {
		t.Fatalf("bad completion: %+v", completed)
	}
	identity, err = f.Config.Identities.GetBySubject(ctx, "google", "new-user")
	if err != nil {
		t.Fatal(err)
	}
	if identity.UserID != completed.Result.Principal.UserID {
		t.Fatal("wrong registered user")
	}

	// Authenticate the newly registered identity through a challenge-based method.
	f.Policy.Intent = entity.IntentSignIn
	f.Policy.Root = entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google"}}}}
	tenant := f.Config.Tenants["demo"]
	tenant.Intents = map[entity.Intent]authenticationservice.IntentConfig{entity.IntentSignIn: {IssueSession: true}}
	f.Config.Tenants["demo"] = tenant
	svc, err = authenticationservice.NewImplementation(f.Config)
	if err != nil {
		t.Fatal(err)
	}
	created, err = svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: entity.IntentSignIn})
	if err != nil {
		t.Fatal(err)
	}
	selection := entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}
	started, err := svc.StartMethod(ctx, application.StartMethodRequest{TenantID: "demo", TransactionID: created.Transaction.ID, Selection: selection, Input: authfixture.Input{}})
	if err != nil {
		t.Fatal(err)
	}
	if started.Challenge == nil || started.RedirectURL == "" {
		t.Fatal("missing challenge or redirect")
	}
	signedIn, err := svc.VerifyMethod(ctx, application.VerifyMethodRequest{TenantID: "demo", TransactionID: created.Transaction.ID, AttemptID: started.AttemptID, ChallengeID: &started.Challenge.ID, Input: authfixture.Input{UserID: "new-user", Identity: true}})
	if err != nil {
		t.Fatal(err)
	}
	if signedIn.Result == nil || signedIn.Result.Principal.UserID != identity.UserID {
		t.Fatal("sign in resolved wrong user")
	}
}

func TestSimpleCallback(t *testing.T) {
	f, _ := signUpFixture(t, false)
	ctx := authfixture.WithCaller(t.Context(), "owner", nil)
	selection := entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}
	var registered entity.UserID
	for _, intent := range []entity.Intent{entity.IntentSignUp, entity.IntentSignIn} {
		t.Run(string(intent), func(t *testing.T) {
			f.Policy.Intent = intent
			tenant := f.Config.Tenants["demo"]
			tenant.Intents = map[entity.Intent]authenticationservice.IntentConfig{intent: {IssueSession: true}}
			f.Config.Tenants["demo"] = tenant
			svc, err := authenticationservice.NewImplementation(f.Config)
			if err != nil {
				t.Fatal(err)
			}
			created, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: intent})
			if err != nil {
				t.Fatal(err)
			}
			started, err := svc.StartMethod(ctx, application.StartMethodRequest{TenantID: "demo", TransactionID: created.Transaction.ID, Selection: selection, Input: authfixture.Input{}})
			if err != nil {
				t.Fatal(err)
			}
			request := application.CompleteCallbackRequest{TenantID: "demo", TransactionID: created.Transaction.ID, AttemptID: started.AttemptID, ChallengeID: started.Challenge.ID, Input: authfixture.Input{Reject: true}}
			if _, err := svc.CompleteCallback(ctx, request); err == nil {
				t.Fatal("rejected callback accepted")
			}
			saved, err := f.Config.Transactions.Get(ctx, "demo", created.Transaction.ID, "owner")
			if err != nil {
				t.Fatal(err)
			}
			if saved.Pending == nil || saved.Claim != nil || len(saved.Evidence) != 0 {
				t.Fatal("failed callback changed transaction")
			}
			request.Input = nil
			decoded := false
			request.DecodeInput = func(actual entity.MethodSelection) (gateway.MethodInput, error) {
				if actual != selection {
					t.Fatal("decoder received wrong selection")
				}
				decoded = true
				return authfixture.Input{UserID: "callback-user", Identity: true}, nil
			}
			wrongAttempt := request
			wrongAttempt.AttemptID = "wrong-attempt"
			if _, err := svc.CompleteCallback(ctx, wrongAttempt); err == nil || decoded {
				t.Fatal("wrong attempt reached decoder")
			}
			completed, err := svc.CompleteCallback(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if !decoded || completed.Result == nil || completed.Result.Session == nil || completed.Transaction.Status != entity.TransactionCompleted {
				t.Fatalf("bad callback result: %+v", completed)
			}
			if intent == entity.IntentSignUp {
				registered = completed.Result.Principal.UserID
			} else if completed.Result.Principal.UserID != registered {
				t.Fatal("sign in resolved wrong user")
			}
			saved, err = f.Config.Transactions.Get(ctx, "demo", created.Transaction.ID, "owner")
			if err != nil {
				t.Fatal(err)
			}
			challenge, err := saved.Challenge(started.Challenge.ID)
			if err != nil {
				t.Fatal(err)
			}
			if challenge.Status != entity.ChallengeConsumed || saved.Pending != nil || len(saved.Evidence) != 1 {
				t.Fatal("callback proof not persisted")
			}
		})
	}
}
