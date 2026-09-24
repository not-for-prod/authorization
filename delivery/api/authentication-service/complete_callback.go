package authenticationservice

import (
	"context"

	"authorization/domain/application"
	"authorization/domain/entity"
	"authorization/domain/gateway"
	pb "authorization/generated/pb/authorization/v1"

	"github.com/not-for-prod/observer/tracer/prospan"
)

func (i *Implementation) CompleteCallback(ctx context.Context, req *pb.CompleteCallbackRequest) (*pb.CompleteCallbackResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if err := i.validate(req); err != nil {
		return nil, span.Err(err)
	}

	tenant := entity.TenantID(req.TenantId)

	response, err := i.app.CompleteCallback(ctx, application.CompleteCallbackRequest{AttemptID: req.AttemptId, TenantID: tenant,
		TransactionID: entity.TransactionID(req.TransactionId), ChallengeID: entity.ChallengeID(req.ChallengeId),
		DecodeInput: func(selection entity.MethodSelection) (gateway.MethodInput, error) {
			return i.decode(ctx, tenant, selection, entity.OperationCallback, req.Input)
		}})
	if err != nil {
		return nil, span.Err(transportError(err))
	}

	return &pb.CompleteCallbackResponse{Transaction: fromTransaction(response.Transaction), Result: fromResult(response.Result)}, nil
}
