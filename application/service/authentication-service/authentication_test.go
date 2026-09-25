package authenticationservice_test

import (
	"context"
	"errors"
	"testing"
	"time"

	authenticationservice "github.com/not-for-prod/authorization/application/service/authentication-service"
	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/internal/testutil/authfixture"

	"github.com/not-for-prod/proterror/proterror"
)

func app(t *testing.T, f *authfixture.Fixture) (*authenticationservice.Implementation, context.Context, entity.TransactionID) {
	t.Helper()

	svc, err := authenticationservice.NewImplementation(f.Config)
	if err != nil {
		t.Fatal(err)
	}

	ctx := authfixture.WithCaller(context.Background(), "browser", nil)

	created, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: "sign_in"})
	if err != nil {
		t.Fatal(err)
	}

	return svc, ctx, created.Transaction.ID
}

func verify(id entity.TransactionID, user string) application.VerifyMethodRequest {
	return application.VerifyMethodRequest{TenantID: "demo", TransactionID: id,
		Input: authfixture.Input{UserID: entity.UserID(user)}}
}

func TestPasswordMFAAndCompletionReplay(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)

	initial, err := svc.GetTransaction(ctx, application.GetTransactionRequest{TenantID: "demo", TransactionID: id})
	if err != nil || len(initial.Transaction.AvailableMethods) != 2 {
		t.Fatalf("initial methods: %+v %v", initial, err)
	}

	writes := f.Store.Writes()

	if _, err := svc.GetTransaction(ctx, application.GetTransactionRequest{TenantID: "demo", TransactionID: id}); err != nil {
		t.Fatal(err)
	}

	if f.Store.Writes() != writes {
		t.Fatal("Get wrote state")
	}

	first, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "primary", "password", "alice"))
	if err != nil || first.Result != nil || first.Transaction.PolicySatisfied {
		t.Fatalf("primary: %+v %v", first, err)
	}

	if f.SessionCount() != 0 || len(first.Transaction.AvailableMethods) != 1 || first.Transaction.AvailableMethods[0].Method != "totp" {
		t.Fatal("incorrect MFA progress")
	}

	// A policy reload cannot reduce the requirements of this transaction.
	f.Policy.Root = entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{entity.MethodRequirement{Method: "password"}}}

	if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "mfa", "totp", "bob")); !errors.Is(err, entity.ErrSubjectMismatch) {
		t.Fatalf("mixed subject: %v", err)
	}

	completionRequest := startVerification(t, svc, ctx, id, "mfa", "totp", "alice")
	completed, err := svc.VerifyMethod(ctx, completionRequest)
	if err != nil || completed.Result == nil || completed.Result.Session == nil {
		t.Fatalf("completion: %+v %v", completed, err)
	}

	calls := len(f.Methods["totp"].VerifyCalls())

	replay, err := svc.VerifyMethod(ctx, completionRequest)
	if err != nil || replay.Result.Session.Token != completed.Result.Session.Token || f.SessionCount() != 1 {
		t.Fatalf("retry: %+v %v", replay, err)
	}

	if len(f.Methods["totp"].VerifyCalls()) != calls {
		t.Fatal("completion retry verified proof again")
	}

	if _, err := svc.VerifyMethod(ctx, verify(id, "alice")); err == nil {
		t.Fatal("unrelated completion operation accepted")
	}
}

func TestCallbackUsesStoredSelectionAndConsumesChallenge(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)
	selection := entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}

	started, err := svc.StartMethod(ctx, application.StartMethodRequest{TenantID: "demo", TransactionID: id, Selection: selection, Input: authfixture.Input{}})
	if err != nil || started.Challenge == nil || started.RedirectURL == "" {
		t.Fatalf("start: %+v %v", started, err)
	}

	decoded := false
	request := application.CompleteCallbackRequest{TenantID: "demo", TransactionID: id, ChallengeID: started.Challenge.ID, AttemptID: started.AttemptID,
		DecodeInput: func(actual entity.MethodSelection) (gateway.MethodInput, error) {
			if actual != selection {
				t.Fatal("decoder received caller-selected method")
			}

			decoded = true

			return authfixture.Input{UserID: "alice", Identity: true}, nil
		}}

	response, err := svc.CompleteCallback(ctx, request)
	if err != nil || !decoded || response.Result != nil {
		t.Fatalf("callback: %+v %v", response, err)
	}

	saved, err := f.Store.Get(ctx, "demo", id, "browser")
	if err != nil {
		t.Fatal(err)
	}

	if saved.Challenges[0].Status != entity.ChallengeConsumed || len(saved.Challenges[0].State) != 0 || len(saved.Evidence) != 1 {
		t.Fatal("challenge/evidence not committed together")
	}

	if _, err := svc.CompleteCallback(ctx, request); err != nil {
		t.Fatal("stored callback replay failed", err)
	}

	if len(f.Methods["oauth"].CallbackCalls()) != 1 {
		t.Fatal("callback replay dispatched")
	}
}

func TestInvalidAccessAndSelection(t *testing.T) {
	for _, name := range []string{"owner", "tenant", "missing transaction", "expired", "unavailable", "missing input", "empty challenge"} {
		t.Run(name, func(t *testing.T) {
			f := authfixture.New()
			svc, ctx, id := app(t, f)
			req := startVerification(t, svc, ctx, id, "primary", "password", "alice")

			switch name {
			case "owner":
				ctx = authfixture.WithCaller(ctx, "other", nil)
			case "tenant":
				req.TenantID = "other"
			case "missing transaction":
				req.TransactionID = "missing"
			case "expired":
				f.Advance(5 * time.Minute)
			case "unavailable":
				req = verify(id, "alice")
			case "missing input":
				req.Input = nil
			case "empty challenge":
				empty := entity.ChallengeID("")
				req.ChallengeID = &empty
			}

			if _, err := svc.VerifyMethod(ctx, req); err == nil {
				t.Fatal("invalid request accepted")
			}

			if f.SessionCount() != 0 || len(f.Methods["password"].VerifyCalls()) != 0 || len(f.Methods["totp"].VerifyCalls()) != 0 {
				t.Fatal("rejected request dispatched")
			}
		})
	}
}

func TestUnknownOutcomeAndReadOnlyRecovery(t *testing.T) {
	f := authfixture.New()
	f.Methods["password"].VerifyFunc = func(context.Context, entity.Transaction, entity.MethodSelection, *entity.Challenge, gateway.MethodInput) (gateway.VerificationResult, error) {
		return gateway.VerificationResult{}, &proterror.Unavailable{}
	}

	svc, ctx, id := app(t, f)

	req := startVerification(t, svc, ctx, id, "primary", "password", "alice")
	if _, err := svc.VerifyMethod(ctx, req); err == nil {
		t.Fatal("uncertain driver result accepted")
	}

	if _, err := svc.VerifyMethod(ctx, req); !errors.Is(err, entity.ErrTransactionBusy) {
		t.Fatalf("expected busy: %v", err)
	}

	f.Advance(11 * time.Second)
	writes := f.Store.Writes()

	if _, err := svc.GetTransaction(ctx, application.GetTransactionRequest{TenantID: "demo", TransactionID: id}); err != nil {
		t.Fatal(err)
	}

	if f.Store.Writes() != writes {
		t.Fatal("Get repaired abandoned claim")
	}

	if _, err := svc.VerifyMethod(ctx, req); !errors.Is(err, entity.ErrTransactionAbandoned) {
		t.Fatalf("expected restart: %v", err)
	}

	saved, err := f.Store.Get(ctx, "demo", id, "browser")
	if err != nil {
		t.Fatal(err)
	}

	if saved.Status != entity.TransactionAbandoned || len(f.Methods["password"].VerifyCalls()) != 1 {
		t.Fatal("unknown outcome repeated")
	}
}

func TestConcurrentVerificationAndLateResult(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "single dispatcher", true: "stale result"}[late], func(t *testing.T) {
			f := authfixture.New()
			entered := make(chan struct{})
			release := make(chan struct{})
			original := f.Methods["password"].VerifyFunc
			f.Methods["password"].VerifyFunc = func(ctx context.Context, tx entity.Transaction, selection entity.MethodSelection, c *entity.Challenge, input gateway.MethodInput) (gateway.VerificationResult, error) {
				close(entered)
				<-release

				return original(ctx, tx, selection, c, input)
			}

			svc, ctx, id := app(t, f)
			req := startVerification(t, svc, ctx, id, "primary", "password", "alice")
			done := make(chan error, 1)

			go func() { _, err := svc.VerifyMethod(ctx, req); done <- err }()

			<-entered

			if _, err := svc.VerifyMethod(ctx, req); !errors.Is(err, entity.ErrTransactionBusy) {
				t.Fatalf("competing call: %v", err)
			}

			if late {
				f.Advance(11 * time.Second)

				if _, err := svc.VerifyMethod(ctx, req); !errors.Is(err, entity.ErrTransactionAbandoned) {
					t.Fatal(err)
				}
			}

			close(release)

			err := <-done
			if late && err == nil {
				t.Fatal("late result accepted")
			}

			if !late && err != nil {
				t.Fatal(err)
			}

			stored, err := f.Store.Get(ctx, "demo", id, "browser")
			if err != nil {
				t.Fatal(err)
			}

			if late && len(stored.Evidence) != 0 {
				t.Fatal("late result persisted")
			}

			if len(f.Methods["password"].VerifyCalls()) != 1 {
				t.Fatal("multiple dispatchers")
			}
		})
	}
}

func TestEvidenceRollback(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)
	f.Store.UpdateError = func(tx entity.Transaction) error {
		if len(tx.Evidence) > 0 {
			return errors.New("save failed")
		}

		return nil
	}

	if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "primary", "password", "alice")); err == nil {
		t.Fatal("failed save accepted")
	}

	saved, err := f.Store.Get(ctx, "demo", id, "browser")
	if err != nil {
		t.Fatal(err)
	}

	if saved.UserID != nil || len(saved.Evidence) != 0 || saved.Claim == nil || f.SessionCount() != 0 {
		t.Fatal("failed transaction did not roll back")
	}
}

func TestSessionRecovery(t *testing.T) {
	for _, failure := range []string{"issuer response", "completion commit"} {
		t.Run(failure, func(t *testing.T) {
			f := authfixture.New()

			svc, ctx, id := app(t, f)
			if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "primary", "password", "alice")); err != nil {
				t.Fatal(err)
			}

			fail := true

			if failure == "issuer response" {
				create := f.Sessions.CreateFunc
				f.Sessions.CreateFunc = func(ctx context.Context, req gateway.CreateSessionRequest) (entity.IssuedSession, error) {
					issued, err := create(ctx, req)

					if fail {
						fail = false
						return entity.IssuedSession{}, &proterror.Unavailable{}
					}

					return issued, err
				}
			} else {
				f.Store.UpdateError = func(tx entity.Transaction) error {
					if fail && tx.Status == entity.TransactionCompleted {
						fail = false
						return errors.New("commit failed")
					}

					return nil
				}
			}

			req := startVerification(t, svc, ctx, id, "mfa", "totp", "alice")
			if _, err := svc.VerifyMethod(ctx, req); err == nil {
				t.Fatal("expected completion failure")
			}

			stored, err := f.Store.Get(ctx, "demo", id, "browser")
			if err != nil {
				t.Fatal(err)
			}

			if stored.Status != entity.TransactionFinalizing || stored.Principal == nil {
				t.Fatal("missing durable completion intent")
			}

			result, err := svc.VerifyMethod(ctx, req)
			if err != nil || result.Result == nil || result.Result.Session == nil {
				t.Fatalf("recovery: %+v %v", result, err)
			}

			if f.SessionCount() != 1 || len(f.Methods["totp"].VerifyCalls()) != 1 {
				t.Fatal("recovery duplicated effect")
			}

			requests := f.Sessions.CreateCalls()
			if len(requests) != 2 || requests[0].Req.Key != requests[1].Req.Key {
				t.Fatal("unstable issuance key")
			}
		})
	}
}

func TestPrincipalOnlyAndTrustedUser(t *testing.T) {
	f := authfixture.New()
	tenant := f.Config.Tenants["demo"]
	tenant.Intents["sign_in"] = authenticationservice.IntentConfig{RequireAuthenticatedUser: true}
	f.Config.Tenants["demo"] = tenant

	svc, err := authenticationservice.NewImplementation(f.Config)
	if err != nil {
		t.Fatal(err)
	}

	ctx := authfixture.WithCaller(context.Background(), "browser", nil)

	user := entity.UserID("alice")
	if _, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: "sign_in", AuthenticatedUserID: &user}); err == nil {
		t.Fatal("caller supplied user trusted")
	}

	ctx = authfixture.WithCaller(ctx, "browser", &user)

	created, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: "sign_in"})
	if err != nil {
		t.Fatal(err)
	}

	id := created.Transaction.ID
	if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "primary", "password", "alice")); err != nil {
		t.Fatal(err)
	}

	result, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "mfa", "totp", "alice"))
	if err != nil || result.Result == nil || result.Result.Session != nil || f.SessionCount() != 0 {
		t.Fatalf("principal-only: %+v %v", result, err)
	}
}

func TestDefinitiveRejectionAndMalformedProof(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)
	req := startVerification(t, svc, ctx, id, "primary", "password", "alice")

	req.Input = authfixture.Input{Reject: true}
	if _, err := svc.VerifyMethod(ctx, req); !errors.Is(err, gateway.ErrProofRejected) {
		t.Fatal(err)
	}

	saved, err := f.Store.Get(ctx, "demo", id, "browser")
	if err != nil {
		t.Fatal(err)
	}

	if saved.Claim != nil || len(saved.Evidence) != 0 {
		t.Fatal("definitive rejection retained claim/evidence")
	}

	original := f.Methods["password"].VerifyFunc
	f.Methods["password"].VerifyFunc = func(ctx context.Context, tx entity.Transaction, s entity.MethodSelection, c *entity.Challenge, input gateway.MethodInput) (gateway.VerificationResult, error) {
		proof, err := original(ctx, tx, s, c, input)
		proof.Evidence.Requirement = "mfa"

		return proof, err
	}

	if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "primary", "password", "alice")); !errors.Is(err, entity.ErrInvalidProof) {
		t.Fatal(err)
	}

	saved, err = f.Store.Get(ctx, "demo", id, "browser")
	if err != nil {
		t.Fatal(err)
	}

	if len(saved.Evidence) != 0 || f.SessionCount() != 0 {
		t.Fatal("malformed driver proof accepted")
	}
}

func TestChallengeVerifyBindingExpiryAndUnknownIdentity(t *testing.T) {
	for _, scenario := range []string{"verify", "wrong transaction", "wrong attempt", "expired challenge", "unknown identity"} {
		t.Run(scenario, func(t *testing.T) {
			f := authfixture.New()
			if scenario == "expired challenge" {
				original := f.Methods["oauth"].StartFunc
				f.Methods["oauth"].StartFunc = func(ctx context.Context, tx entity.Transaction, s entity.MethodSelection, input gateway.MethodInput) (gateway.StartResult, error) {
					r, err := original(ctx, tx, s, input)
					r.Challenge.ExpiresAt = f.Now().Add(time.Second)

					return r, err
				}
			}

			svc, ctx, id := app(t, f)
			selection := entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}

			started, err := svc.StartMethod(ctx, application.StartMethodRequest{TenantID: "demo", TransactionID: id, Selection: selection, Input: authfixture.Input{}})
			if err != nil {
				t.Fatal(err)
			}

			req := application.VerifyMethodRequest{TenantID: "demo", TransactionID: id, ChallengeID: &started.Challenge.ID, AttemptID: started.AttemptID, Input: authfixture.Input{UserID: "alice", Identity: true}}

			switch scenario {
			case "wrong transaction":
				other, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: "sign_in"})
				if err != nil {
					t.Fatal(err)
				}

				req.TransactionID = other.Transaction.ID
			case "wrong attempt":
				req.AttemptID = "other-attempt"
			case "expired challenge":
				f.Advance(time.Second)
			case "unknown identity":
				req.Input = authfixture.Input{UserID: "unregistered", Identity: true}
			}

			result, err := svc.VerifyMethod(ctx, req)
			if scenario == "verify" {
				if err != nil || result.Result != nil {
					t.Fatalf("challenged verify: %+v %v", result, err)
				}
			} else if err == nil {
				t.Fatal("invalid challenge or subject accepted")
			}

			if scenario == "unknown identity" && (len(f.Users.CreateCalls()) != 0 || len(f.Identities.CreateCalls()) != 0) {
				t.Fatal("registered an unknown subject")
			}

			if f.SessionCount() != 0 {
				t.Fatal("session before MFA")
			}
		})
	}
}

func TestStartValidationAndUnknownOutcome(t *testing.T) {
	for _, scenario := range []string{"no challenge", "oversized state", "wrong transaction", "expired", "unknown outcome", "definitive rejection"} {
		t.Run(scenario, func(t *testing.T) {
			f := authfixture.New()
			original := f.Methods["oauth"].StartFunc
			f.Methods["oauth"].StartFunc = func(ctx context.Context, tx entity.Transaction, s entity.MethodSelection, input gateway.MethodInput) (gateway.StartResult, error) {
				result, err := original(ctx, tx, s, input)

				switch scenario {
				case "no challenge":
					result.Challenge = nil
					result.RedirectURL = ""
				case "oversized state":
					result.Challenge.State = make([]byte, 5000)
				case "wrong transaction":
					result.Challenge.TransactionID = "other"
				case "expired":
					result.Challenge.ExpiresAt = f.Now()
				case "unknown outcome":
					return gateway.StartResult{}, &proterror.Unavailable{}
				case "definitive rejection":
					return gateway.StartResult{}, gateway.ErrProofRejected
				}

				return result, err
			}

			svc, ctx, id := app(t, f)
			req := application.StartMethodRequest{TenantID: "demo", TransactionID: id, Selection: entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}, Input: authfixture.Input{}}

			_, err := svc.StartMethod(ctx, req)
			if scenario == "no challenge" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid start succeeded")
			}

			saved, err := f.Store.Get(ctx, "demo", id, "browser")
			if err != nil {
				t.Fatal(err)
			}

			if len(saved.Challenges) != 0 {
				t.Fatal("invalid challenge persisted")
			}

			if scenario == "unknown outcome" && saved.Claim == nil {
				t.Fatal("uncertain start released claim")
			}

			if scenario == "definitive rejection" && saved.Claim != nil {
				t.Fatal("definitive start rejection retained claim")
			}
		})
	}
}

func TestCancellationAfterDispatchDoesNotAcceptEvidence(t *testing.T) {
	f := authfixture.New()
	svc, ctx, id := app(t, f)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	original := f.Methods["password"].VerifyFunc
	f.Methods["password"].VerifyFunc = func(ctx context.Context, tx entity.Transaction, s entity.MethodSelection, c *entity.Challenge, input gateway.MethodInput) (gateway.VerificationResult, error) {
		cancel()
		return original(ctx, tx, s, c, input)
	}

	if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "primary", "password", "alice")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	saved, err := f.Store.Get(context.Background(), "demo", id, "browser")
	if err != nil {
		t.Fatal(err)
	}

	if len(saved.Evidence) != 0 || saved.Claim == nil {
		t.Fatal("canceled dispatch was accepted or retried")
	}
}

func TestEligibilityBeforeIssuanceAndSessionValidation(t *testing.T) {
	for _, scenario := range []string{"ineligible user", "wrong session principal", "expired session"} {
		t.Run(scenario, func(t *testing.T) {
			f := authfixture.New()
			deny := false
			eligible := f.Config.Eligible
			f.Config.Eligible = func(ctx context.Context, tenant entity.TenantID, user entity.User) error {
				if deny {
					return &proterror.PermissionDenied{}
				}

				return eligible(ctx, tenant, user)
			}

			svc, ctx, id := app(t, f)
			if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "primary", "password", "alice")); err != nil {
				t.Fatal(err)
			}

			if scenario == "ineligible user" {
				deny = true
			} else {
				create := f.Sessions.CreateFunc
				f.Sessions.CreateFunc = func(ctx context.Context, req gateway.CreateSessionRequest) (entity.IssuedSession, error) {
					r, err := create(ctx, req)
					if scenario == "wrong session principal" {
						r.Session.Principal.UserID = "bob"
					} else {
						r.Session.ExpiresAt = f.Now()
					}

					return r, err
				}
			}

			if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "mfa", "totp", "alice")); err == nil {
				t.Fatal("invalid completion accepted")
			}

			saved, err := f.Store.Get(ctx, "demo", id, "browser")
			if err != nil {
				t.Fatal(err)
			}

			if saved.Status == entity.TransactionCompleted {
				t.Fatal("invalid session completed transaction")
			}

			if scenario == "ineligible user" && f.SessionCount() != 0 {
				t.Fatal("ineligible user received session")
			}
		})
	}
}

func TestExpiryBeforeSessionDispatch(t *testing.T) {
	f := authfixture.New()
	checks := 0
	eligible := f.Config.Eligible
	f.Config.Eligible = func(ctx context.Context, tenant entity.TenantID, user entity.User) error {
		checks++
		// Primary binding, MFA binding, then final issuance eligibility.
		if checks == 3 {
			f.Advance(5 * time.Minute)
		}

		return eligible(ctx, tenant, user)
	}

	svc, ctx, id := app(t, f)
	if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "primary", "password", "alice")); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.VerifyMethod(ctx, startVerification(t, svc, ctx, id, "mfa", "totp", "alice")); !errors.Is(err, entity.ErrTransactionExpired) {
		t.Fatal(err)
	}

	if len(f.Sessions.CreateCalls()) != 0 {
		t.Fatal("issued session after authentication expiry")
	}
}

// startVerification makes the mandatory Start step explicit in each test setup.
func startVerification(t *testing.T, svc application.Authentication, ctx context.Context, id entity.TransactionID, requirement, method, user string) application.VerifyMethodRequest {
	t.Helper()
	req := verify(id, user)
	started, err := svc.StartMethod(ctx, application.StartMethodRequest{TenantID: req.TenantID, TransactionID: id, Selection: entity.MethodSelection{Requirement: entity.RequirementID(requirement), Method: entity.MethodID(method)}, Input: authfixture.Input{}})
	if err != nil {
		t.Fatal(err)
	}
	req.AttemptID = started.AttemptID
	return req
}
