package repository

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Policy provides a requirement tree for exactly one tenant and intent. It may
// be backed by host configuration or persistent storage. Missing keys return
// ErrNotFound; no fallback to another tenant or intent is permitted.
//
//go:generate moq -out ../../generated/mocks/repository/policy.go -pkg repositorymock . Policy
type Policy interface {
	Get(ctx context.Context, tenantID entity.TenantID, intent entity.Intent) (*entity.Policy, error)
}
