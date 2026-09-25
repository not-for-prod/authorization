package authenticationservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
	repositorymock "github.com/not-for-prod/authorization/generated/mocks/repository"
)

func TestSharedBindingDefersIntentRulesToFinalization(t *testing.T) {
	for _, intent := range []entity.Intent{entity.IntentSignIn, entity.IntentSignUp} {
		for _, registered := range []bool{false, true} {
			t.Run(string(intent)+map[bool]string{false: "/new", true: "/existing"}[registered], func(t *testing.T) {
				now := time.Now().UTC()
				createdUsers, createdIdentities, eligibilityChecks := 0, 0, 0
				users := &repositorymock.UserMock{
					GetFunc: func(_ context.Context, id entity.UserID, _ ...repository.SelectOption) (*entity.User, error) {
						return &entity.User{ID: id, Status: entity.UserStatusActive}, nil
					},
					CreateFunc: func(context.Context, entity.User) error { createdUsers++; return nil },
				}
				identities := &repositorymock.IdentityMock{
					GetBySubjectFunc: func(_ context.Context, issuer, subject string, _ ...repository.SelectOption) (*entity.Identity, error) {
						if !registered {
							return nil, repository.ErrNotFound
						}
						return &entity.Identity{Kind: "oauth", Issuer: issuer, Subject: subject, UserID: "alice"}, nil
					},
					CreateFunc: func(context.Context, entity.Identity) error { createdIdentities++; return nil },
				}
				svc := &Implementation{users: users, identities: identities, now: func() time.Time { return now }, eligible: func(context.Context, entity.TenantID, entity.User) error { eligibilityChecks++; return nil }}
				selection := entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}
				policy := entity.Policy{TenantID: "tenant", Intent: intent, Root: entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google"}}}}}
				tx, err := entity.NewTransaction("tx", "owner", policy, nil, entity.TransactionSettings{Lifetime: time.Minute, OperationTimeout: time.Second, MaxAttempts: 4, MaxChallenges: 2, MaxChallengeBytes: 32, MaxPolicyNodes: 10}, now)
				if err != nil {
					t.Fatal(err)
				}
				if err := tx.ClaimOperation("start", entity.OperationStart, selection, "", now); err != nil {
					t.Fatal(err)
				}
				if err := tx.RecordMethodStarted("start", nil, "", now); err != nil {
					t.Fatal(err)
				}
				if err := tx.ClaimOperation("verify", entity.OperationVerify, selection, "", now); err != nil {
					t.Fatal(err)
				}
				proof := gateway.VerificationResult{Identity: &gateway.VerifiedIdentity{Kind: "oauth", Issuer: "google", Subject: "alice"}, Evidence: entity.Evidence{Requirement: "primary", Method: "oauth", Provider: "google", VerifiedAt: now}}
				userID, identity, err := svc.resolveVerifiedSubject(t.Context(), proof)
				if err != nil {
					t.Fatal(err)
				}
				decision, err := tx.AcceptMethodProof(t.Context(), svc.evaluator, "verify", userID, identity, proof.Evidence, now)
				if err != nil || !decision.Satisfied {
					t.Fatal("proof acceptance applied intent rules", err)
				}
				if createdUsers != 0 || createdIdentities != 0 || eligibilityChecks != 0 {
					t.Fatal("finalization ran during binding")
				}
				var result application.AuthenticationResult
				err = svc.finalizeIntent(t.Context(), finalizeIntentRequest{Transaction: tx, Decision: decision, Result: &result})
				switch {
				case intent == entity.IntentSignIn && !registered:
					if !errors.Is(err, repository.ErrNotFound) {
						t.Fatal("sign-in must require an existing user", err)
					}
				case intent == entity.IntentSignUp && registered:
					if !errors.Is(err, repository.ErrAlreadyExists) {
						t.Fatal("sign-up must reject registered subject", err)
					}
				default:
					if err != nil {
						t.Fatal(err)
					}
					if tx.Status != entity.TransactionCompleted || result.Principal.UserID == "" || eligibilityChecks != 1 {
						t.Fatal("incomplete finalization")
					}
				}
				wantCreates := 0
				if intent == entity.IntentSignUp && !registered {
					wantCreates = 1
				}
				if createdUsers != wantCreates || createdIdentities != wantCreates {
					t.Fatal("unexpected account writes")
				}
			})
		}
	}
}
