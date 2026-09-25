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
	gatewaymock "github.com/not-for-prod/authorization/generated/mocks/gateway"
	repositorymock "github.com/not-for-prod/authorization/generated/mocks/repository"
)

type callbackLeaseEvents struct{}

func (callbackLeaseEvents) Publish(context.Context, []entity.TransactionEvent) error { return nil }

type callbackLeaseInput struct{}

func (callbackLeaseInput) AuthenticationInput() {}

func TestCallbackDoesNotPersistDispatchLease(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncertain", true: "rejected"}[rejected], func(t *testing.T) {
			now := time.Now().UTC()
			selection := entity.MethodSelection{Requirement: "primary", Method: "oauth"}
			tx := entity.Transaction{
				ID: "tx", TenantID: "tenant", Intent: entity.IntentSignIn, OwnerKey: "owner",
				Status: entity.TransactionActive, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
				Settings:   entity.TransactionSettings{MaxAttempts: 5, OperationTimeout: time.Second},
				Pending:    &entity.MethodAttempt{ID: "attempt", Selection: selection, ChallengeID: "challenge", Ready: true},
				Challenges: []entity.Challenge{{ID: "challenge", TransactionID: "tx", Selection: selection, Status: entity.ChallengePending, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}},
			}
			source := errors.New("provider unavailable")
			if rejected {
				source = gateway.ErrProofRejected
			}
			writes, transactions := 0, 0
			svc := &Implementation{
				now: func() time.Time { return now },
				access: func(context.Context, entity.TenantID) (application.Caller, error) {
					return application.Caller{Key: "owner"}, nil
				},
				transactions: &repositorymock.TransactionMock{
					GetFunc: func(_ context.Context, _ entity.TenantID, _ entity.TransactionID, owner string, opts ...repository.SelectOption) (*entity.Transaction, error) {
						if owner != "owner" {
							t.Fatal("missing ownership filter")
						}
						if transactions == 0 && len(opts) != 0 {
							t.Fatal("dispatch read acquired write lock")
						}
						if transactions > 0 && (len(opts) != 1 || opts[0] != repository.SelectForUpdate) {
							t.Fatal("persistence missing write lock")
						}
						copy := tx.Copy()
						return &copy, nil
					},
					UpdateFunc: func(_ context.Context, saved entity.Transaction) error {
						writes++
						if saved.Claim != nil {
							t.Fatal("persisted callback lease")
						}
						tx = saved
						return nil
					},
				},
				txManager: &repositorymock.TransactionManagerMock{DoFunc: func(ctx context.Context, operation func(context.Context) error) error {
					transactions++
					return operation(ctx)
				}},
				events: callbackLeaseEvents{},
				tenants: map[entity.TenantID]tenantConfig{"tenant": {methods: map[entity.MethodID]gateway.Method{
					"oauth": &gatewaymock.CallbackMethodMock{CallbackFunc: func(_ context.Context, snapshot entity.Transaction, _ entity.MethodSelection, _ entity.Challenge, _ gateway.MethodInput) (gateway.VerificationResult, error) {
						if transactions != 0 || writes != 0 || snapshot.Claim != nil {
							t.Fatal("lease persisted before provider call")
						}
						now = now.Add(2 * time.Second)
						return gateway.VerificationResult{}, source
					}},
				}}},
			}
			_, err := svc.CompleteCallback(t.Context(), application.CompleteCallbackRequest{TenantID: "tenant", TransactionID: "tx", AttemptID: "attempt", ChallengeID: "challenge", Input: callbackLeaseInput{}})
			if err != source {
				t.Fatalf("provider error changed: %v", err)
			}
			want := 0
			if rejected {
				want = 1
			}
			if writes != want || transactions != want {
				t.Fatalf("writes=%d transactions=%d want=%d", writes, transactions, want)
			}
			if tx.Claim != nil || tx.Status != entity.TransactionActive {
				t.Fatal("callback left lease or abandoned transaction")
			}
		})
	}
}
