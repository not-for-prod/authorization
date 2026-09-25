package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/domain/service"
	gatewaymock "github.com/not-for-prod/authorization/generated/mocks/gateway"
	repositorymock "github.com/not-for-prod/authorization/generated/mocks/repository"
)

type demoMethodInput struct {
	Proof string
}

func (demoMethodInput) AuthenticationInput() {}

// TestAuthenticationWithMocks demonstrates the contracts and real evaluator.
// State changes below are test setup, not an application implementation or proof
// of atomic persistence, identity resolution, challenge consumption, or retries.
func TestAuthenticationWithMocks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	userID := entity.UserID("demo_user")
	policyRepo := &repositorymock.PolicyMock{
		GetFunc: func(_ context.Context, tenantID entity.TenantID, intent entity.Intent) (*entity.Policy, error) {
			if tenantID != "demo" || intent != "sign_in" {
				return nil, repository.ErrNotFound
			}

			return &entity.Policy{
				TenantID: tenantID,
				Intent:   intent,
				Root: entity.AllOfRequirement{Children: []entity.Requirement{
					entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
						entity.MethodRequirement{Method: "password"},
					}},
					entity.AnyOfRequirement{ID: "mfa", Children: []entity.Requirement{
						entity.MethodRequirement{Method: "totp"},
					}},
				}},
			}, nil
		},
	}

	transactionRepo := &repositorymock.TransactionMock{
		GetFunc: func(
			_ context.Context, tenantID entity.TenantID, id entity.TransactionID, ownerKey string, opts ...repository.SelectOption,
		) (*entity.Transaction, error) {
			if tenantID != "demo" || id != "demo_transaction" || ownerKey != "owner" {
				return nil, repository.ErrNotFound
			}

			if len(opts) != 0 {
				return nil, errors.New("demo supports unlocked reads only")
			}

			return &entity.Transaction{
				ID: id, TenantID: tenantID, OwnerKey: ownerKey, Intent: "sign_in",
				CreatedAt: now, ExpiresAt: now.Add(5 * time.Minute),
			}, nil
		},
	}

	password := &gatewaymock.MethodMock{
		VerifyFunc: func(
			_ context.Context, tx entity.Transaction, selection entity.MethodSelection,
			challenge *entity.Challenge, input gateway.MethodInput,
		) (gateway.VerificationResult, error) {
			proof, ok := input.(demoMethodInput)
			if !ok || proof.Proof != "demo-password" || tx.TenantID != "demo" || challenge != nil ||
				selection != (entity.MethodSelection{Requirement: "primary", Method: "password"}) {
				return gateway.VerificationResult{}, errors.New("invalid demo password input")
			}

			return gateway.VerificationResult{
				UserID: &userID,
				Evidence: entity.Evidence{
					Requirement: "primary", Method: "password", VerifiedAt: now,
				},
			}, nil
		},
	}

	mfa := &gatewaymock.MethodMock{
		AvailableFunc: func(_ context.Context, tx entity.Transaction, selection entity.MethodSelection) (bool, error) {
			return tx.TenantID == "demo" && tx.UserID != nil && *tx.UserID == userID &&
				selection == (entity.MethodSelection{Requirement: "mfa", Method: "totp"}), nil
		},
		VerifyFunc: func(
			_ context.Context, tx entity.Transaction, selection entity.MethodSelection,
			challenge *entity.Challenge, input gateway.MethodInput,
		) (gateway.VerificationResult, error) {
			proof, ok := input.(demoMethodInput)
			if !ok || proof.Proof != "123456" || tx.TenantID != "demo" ||
				tx.UserID == nil || *tx.UserID != userID || challenge != nil ||
				selection != (entity.MethodSelection{Requirement: "mfa", Method: "totp"}) {
				return gateway.VerificationResult{}, errors.New("invalid demo TOTP input")
			}

			return gateway.VerificationResult{
				UserID: &userID,
				Evidence: entity.Evidence{
					Requirement: "mfa", Method: "totp", VerifiedAt: now,
				},
			}, nil
		},
	}

	sessions := &gatewaymock.SessionManagerMock{
		CreateFunc: func(_ context.Context, req gateway.CreateSessionRequest) (entity.IssuedSession, error) {
			return entity.IssuedSession{
				Session: entity.Session{
					ID: "demo_session", Principal: req.Principal,
					IssuedAt: now, ExpiresAt: now.Add(time.Hour),
				},
				Token: "demo-token-not-a-real-credential",
			}, nil
		},
	}

	tx, err := transactionRepo.Get(ctx, "demo", "demo_transaction", "owner")
	if err != nil {
		t.Fatal(err)
	}

	policy, err := policyRepo.Get(ctx, tx.TenantID, tx.Intent)
	if err != nil {
		t.Fatal(err)
	}

	evaluator := service.Evaluator{}

	decision, err := evaluator.Evaluate(ctx, *tx, *policy)
	if err != nil || decision.Satisfied {
		t.Fatalf("initial evaluation: satisfied=%v, err=%v", decision.Satisfied, err)
	}

	mfaSelection := entity.MethodSelection{Requirement: "mfa", Method: "totp"}

	available, err := mfa.Available(ctx, *tx, mfaSelection)
	if err != nil || available {
		t.Fatalf("MFA before identification: available=%v, err=%v", available, err)
	}

	t.Log("new authentication transaction: policy unsatisfied; MFA unavailable until a user is identified")

	primary, err := password.Verify(ctx, *tx,
		entity.MethodSelection{Requirement: "primary", Method: "password"}, nil,
		demoMethodInput{Proof: "demo-password"})
	if err != nil {
		t.Fatal(err)
	}

	tx.UserID = primary.UserID
	tx.Evidence = append(tx.Evidence, primary.Evidence)

	decision, err = evaluator.Evaluate(ctx, *tx, *policy)
	if err != nil || decision.Satisfied || decision.Remaining == nil {
		t.Fatalf("password evaluation: decision=%+v, err=%v", decision, err)
	}

	available, err = mfa.Available(ctx, *tx, mfaSelection)
	if err != nil || !available {
		t.Fatalf("MFA after identification: available=%v, err=%v", available, err)
	}

	t.Log("password accepted: user identified; MFA required and available")

	second, err := mfa.Verify(ctx, *tx, mfaSelection, nil, demoMethodInput{Proof: "123456"})
	if err != nil {
		t.Fatal(err)
	}

	if second.UserID == nil || tx.UserID == nil || *second.UserID != *tx.UserID {
		t.Fatal("proofs must identify the same user")
	}

	tx.Evidence = append(tx.Evidence, second.Evidence)

	decision, err = evaluator.Evaluate(ctx, *tx, *policy)
	if err != nil || !decision.Satisfied || decision.Remaining != nil {
		t.Fatalf("MFA evaluation: decision=%+v, err=%v", decision, err)
	}

	if len(sessions.CreateCalls()) != 0 {
		t.Fatal("session issuance preceded policy satisfaction")
	}

	t.Log("TOTP accepted: policy satisfied")

	issued, err := sessions.Create(ctx, gateway.CreateSessionRequest{Key: tx.ID, RecoverUntil: tx.ExpiresAt, Principal: entity.Principal{
		TenantID: tx.TenantID, UserID: *tx.UserID,
		Evidence: slices.Clone(tx.Evidence), AuthenticatedAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}

	if issued.Token == "" || issued.Session.Principal.UserID != userID ||
		issued.Session.Principal.TenantID != tx.TenantID ||
		!slices.Equal(issued.Session.Principal.Evidence, tx.Evidence) {
		t.Fatal("session must contain a credential and the verified principal")
	}

	if len(policyRepo.GetCalls()) != 1 || len(transactionRepo.GetCalls()) != 1 ||
		len(password.VerifyCalls()) != 1 || len(mfa.VerifyCalls()) != 1 || len(sessions.CreateCalls()) != 1 {
		t.Fatal("unexpected dependency calls")
	}

	t.Log("demo session issued for the verified principal")
}
