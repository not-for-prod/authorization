package authenticationservice

import (
	"context"

	"github.com/not-for-prod/observer/tracer/prospan"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	pb "github.com/not-for-prod/authorization/generated/pb/authorization/v1"
)

// Policies returned by the application are bounded value-node snapshots.
func fromPolicyRequirement(node entity.Requirement) (*pb.PolicyRequirement, error) {
	result := &pb.PolicyRequirement{}

	var children []entity.Requirement

	all := false

	switch n := node.(type) {
	case entity.MethodRequirement:
		if len(n.Providers) <= 1 {
			result.Method = string(n.Method)
			if len(n.Providers) == 1 {
				result.Provider = string(n.Providers[0])
			}
		} else {
			for _, provider := range n.Providers {
				result.Any = append(result.Any, &pb.PolicyRequirement{Method: string(n.Method), Provider: string(provider)})
			}
		}

		return result, nil
	case entity.AnyOfRequirement:
		result.Id = string(n.ID)
		children = n.Children
	case entity.AllOfRequirement:
		result.Id = string(n.ID)
		children = n.Children
		all = true
	default:
		return nil, status.Error(codes.Internal, "invalid policy response")
	}

	for _, child := range children {
		mapped, err := fromPolicyRequirement(child)
		if err != nil {
			return nil, err
		}

		if all {
			result.All = append(result.All, mapped)
		} else {
			result.Any = append(result.Any, mapped)
		}
	}

	return result, nil
}

func (i *Implementation) GetAuthPolicy(ctx context.Context, req *pb.GetAuthPolicyRequest) (*pb.GetAuthPolicyResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if err := i.validate(req); err != nil {
		return nil, span.Err(err)
	}

	response, err := i.app.GetAuthPolicy(ctx, application.GetAuthPolicyRequest{TenantID: entity.TenantID(req.TenantId)})
	if err != nil {
		return nil, span.Err(transportError(err))
	}

	result := &pb.GetAuthPolicyResponse{Intents: make([]*pb.IntentPolicy, 0, len(response.Intents))}
	for _, policy := range response.Intents {
		requirements, err := fromPolicyRequirement(policy.Root)
		if err != nil {
			return nil, span.Err(err)
		}

		result.Intents = append(result.Intents, &pb.IntentPolicy{Intent: intentToProto(policy.Intent), Requirements: requirements})
	}

	return result, nil
}
