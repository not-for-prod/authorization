package authenticationservice_test

import (
	"errors"
	"testing"

	authenticationservice "authorization/application/service/authentication-service"
	"authorization/domain/application"
	"authorization/domain/entity"
	"authorization/domain/gateway"
	"authorization/internal/testutil/authfixture"
)

func TestVerifyStoredSelection(t *testing.T) {
	for _, method := range []string{"password", "oauth"} {
		t.Run(method, func(t *testing.T) {
			f, _ := signUpFixture(t, false)
			svc, err := authenticationservice.NewImplementation(f.Config)
			if err != nil {
				t.Fatal(err)
			}
			ctx := authfixture.WithCaller(t.Context(), "owner", nil)
			created, err := svc.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: "demo", Intent: entity.IntentSignUp})
			if err != nil {
				t.Fatal(err)
			}
			selection := entity.MethodSelection{Requirement: "primary", Method: entity.MethodID(method)}
			if method == "oauth" {
				selection.Provider = "google"
			}
			started, err := svc.StartMethod(ctx, application.StartMethodRequest{TenantID: "demo", TransactionID: created.Transaction.ID, Selection: selection, Input: authfixture.Input{}})
			if err != nil {
				t.Fatal(err)
			}
			decoded := false
			req := application.VerifyMethodRequest{TenantID: "demo", TransactionID: created.Transaction.ID, AttemptID: started.AttemptID,
				DecodeInput: func(actual entity.MethodSelection) (gateway.MethodInput, error) {
					decoded = true
					if actual != selection {
						t.Fatalf("selection = %+v, want %+v", actual, selection)
					}
					return authfixture.Input{UserID: "new-user", Identity: true}, nil
				},
			}
			if started.Challenge != nil {
				req.ChallengeID = &started.Challenge.ID
			}
			wrong := req
			wrong.AttemptID = "other-attempt"
			if _, err := svc.VerifyMethod(ctx, wrong); !errors.Is(err, entity.ErrInvalidTransition) || decoded {
				t.Fatalf("wrong attempt reached decoding: %v", err)
			}
			otherCaller := authfixture.WithCaller(t.Context(), "other-owner", nil)
			if _, err := svc.VerifyMethod(otherCaller, req); err == nil || decoded {
				t.Fatal("wrong owner reached decoding")
			}
			invalid := req
			invalid.DecodeInput = func(entity.MethodSelection) (gateway.MethodInput, error) { return nil, gateway.ErrProofRejected }
			if _, err := svc.VerifyMethod(ctx, invalid); !errors.Is(err, gateway.ErrProofRejected) {
				t.Fatalf("decoder error: %v", err)
			}
			invalid.DecodeInput = func(entity.MethodSelection) (gateway.MethodInput, error) { return nil, nil }
			if _, err := svc.VerifyMethod(ctx, invalid); err == nil {
				t.Fatal("nil decoded input accepted")
			}
			result, err := svc.VerifyMethod(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			if !decoded || result.Result == nil || result.Transaction.Status != entity.TransactionCompleted {
				t.Fatalf("verification failed: %+v", result)
			}
			decoded = false
			if _, err := svc.VerifyMethod(ctx, req); err == nil || decoded {
				t.Fatal("consumed attempt reached decoding")
			}
		})
	}
}
