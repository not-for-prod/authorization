package authenticationservice

import (
	"context"
	"errors"
	"testing"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	repositorymock "github.com/not-for-prod/authorization/generated/mocks/repository"

	"github.com/not-for-prod/proterror/proterror"
)

func TestRepositoryErrorsPropagateUnchanged(t *testing.T) {
	for _, source := range []error{
		repository.ErrNotFound,
		repository.ErrAlreadyExists,
		errors.Join(&proterror.NotFound{}, repository.ErrNotFound),
		errors.Join(&proterror.AlreadyExists{}, repository.ErrAlreadyExists),
	} {
		t.Run(source.Error(), func(t *testing.T) {
			svc := &Implementation{
				access: func(context.Context, entity.TenantID) (application.Caller, error) {
					return application.Caller{Key: "owner"}, nil
				},
				tenants: map[entity.TenantID]tenantConfig{
					"tenant": {intents: map[entity.Intent]IntentConfig{entity.IntentSignIn: {}}},
				},
				policies: &repositorymock.PolicyMock{
					GetFunc: func(context.Context, entity.TenantID, entity.Intent) (*entity.Policy, error) {
						return nil, source
					},
				},
				transactions: &repositorymock.TransactionMock{
					GetFunc: func(context.Context, entity.TenantID, entity.TransactionID, string, ...repository.SelectOption) (*entity.Transaction, error) {
						return nil, source
					},
				},
			}
			_, err := svc.GetAuthPolicy(t.Context(), application.GetAuthPolicyRequest{TenantID: "tenant"})
			if err != source {
				t.Fatalf("GetAuthPolicy changed repository error: %v", err)
			}
			_, err = svc.CreateTransaction(t.Context(), application.CreateTransactionRequest{TenantID: "tenant", Intent: entity.IntentSignIn})
			if err != source {
				t.Fatalf("CreateTransaction changed repository error: %v", err)
			}
			_, err = svc.GetTransaction(t.Context(), application.GetTransactionRequest{TenantID: "tenant", TransactionID: "tx"})
			if err != source {
				t.Fatalf("GetTransaction changed repository error: %v", err)
			}
		})
	}
}
