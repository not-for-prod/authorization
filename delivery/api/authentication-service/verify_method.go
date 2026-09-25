package authenticationservice

import (
	"context"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	pb "github.com/not-for-prod/authorization/generated/pb/authorization/v1"

	"github.com/not-for-prod/observer/tracer/prospan"
)

func (i *Implementation) VerifyMethod(ctx context.Context, req *pb.VerifyMethodRequest) (*pb.VerifyMethodResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if err := i.validate(req); err != nil {
		return nil, span.Err(err)
	}

	tenant := entity.TenantID(req.TenantId)
	request := application.VerifyMethodRequest{
		AttemptID: req.AttemptId, TenantID: tenant,
		TransactionID: entity.TransactionID(req.TransactionId),
		DecodeInput: func(selection entity.MethodSelection) (gateway.MethodInput, error) {
			return i.decode(ctx, tenant, selection, entity.OperationVerify, req.Input)
		},
	}
	if req.ChallengeId != nil {
		id := entity.ChallengeID(*req.ChallengeId)
		request.ChallengeID = &id
	}

	response, err := i.app.VerifyMethod(ctx, request)
	if err != nil {
		return nil, span.Err(transportError(err))
	}

	return &pb.VerifyMethodResponse{Transaction: fromTransaction(response.Transaction), Result: fromResult(response.Result)}, nil
}
