package repository

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Challenge is an optional read projection of a transaction-owned attempt.
// All writes go through Transaction under the owning root lock.
//
//go:generate moq -out ../../generated/mocks/repository/challenge.go -pkg repositorymock . Challenge
type Challenge interface {
	Get(ctx context.Context, tenantID entity.TenantID, transactionID entity.TransactionID, id entity.ChallengeID) (*entity.Challenge, error)
}
