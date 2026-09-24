package configuredpolicy

import (
	"context"
	"errors"

	"github.com/not-for-prod/proterror/proterror"

	"authorization/domain/entity"
	"authorization/domain/repository"
)

func (i *Implementation) Get(ctx context.Context, tenant entity.TenantID, intent entity.Intent) (*entity.Policy, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	policy, ok := i.policies[key{tenant: tenant, intent: intent}]
	if !ok {
		return nil, errors.Join(&proterror.NotFound{}, repository.ErrNotFound)
	}

	snapshot, err := policy.Snapshot(i.maxNodes)
	if err != nil {
		return nil, errors.Join(&proterror.Internal{}, err)
	}

	return &snapshot, nil
}
