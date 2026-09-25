package authenticationservice

import (
	"context"
	"errors"
	"slices"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
)

func (i *Implementation) GetAuthPolicy(ctx context.Context, req application.GetAuthPolicyRequest) (*application.GetAuthPolicyResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if err := req.Verify(); err != nil {
		return nil, span.Err(err)
	}

	if err := ctx.Err(); err != nil {
		return nil, span.Err(err)
	}
	caller, err := i.access(ctx, req.TenantID)
	if err != nil {
		return nil, span.Err(err)
	}
	if caller.Key == "" || (caller.UserID != nil && *caller.UserID == "") {
		return nil, span.Err(&proterror.Unauthenticated{})
	}
	if _, ok := i.tenants[req.TenantID]; !ok {
		return nil, span.Err(&proterror.NotFound{})
	}

	intents := make([]entity.Intent, 0, len(i.tenants[req.TenantID].intents))
	for intent, config := range i.tenants[req.TenantID].intents {
		if !config.RequireAuthenticatedUser || caller.UserID != nil {
			intents = append(intents, intent)
		}
	}
	slices.Sort(intents)

	result := &application.GetAuthPolicyResponse{Intents: make([]entity.Policy, 0, len(intents))}
	for _, intent := range intents {
		policy, err := i.policies.Get(ctx, req.TenantID, intent)
		if err != nil {
			return nil, span.Err(err)
		}
		if policy == nil || policy.TenantID != req.TenantID || policy.Intent != intent {
			return nil, span.Err(&proterror.FailedPrecondition{})
		}
		snapshot, err := policy.Snapshot(i.limits.MaxPolicyNodes)
		if err != nil {
			return nil, span.Err(errors.Join(&proterror.FailedPrecondition{}, err))
		}
		result.Intents = append(result.Intents, snapshot)
	}
	return result, nil
}
