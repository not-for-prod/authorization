package authenticationservice

import (
	"context"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	pb "github.com/not-for-prod/authorization/generated/pb/authorization/v1"

	"github.com/not-for-prod/observer/tracer/prospan"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (i *Implementation) StartMethod(ctx context.Context, req *pb.StartMethodRequest) (*pb.StartMethodResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if err := i.validate(req); err != nil {
		return nil, span.Err(err)
	}

	selection := toSelection(req.Selection)

	input, err := i.decode(ctx, entity.TenantID(req.TenantId), selection, entity.OperationStart, req.Input)
	if err != nil {
		return nil, span.Err(transportError(err))
	}

	response, err := i.app.StartMethod(ctx, application.StartMethodRequest{TenantID: entity.TenantID(req.TenantId),
		TransactionID: entity.TransactionID(req.TransactionId), Selection: selection, Input: input})
	if err != nil {
		return nil, span.Err(transportError(err))
	}

	result := &pb.StartMethodResponse{AttemptId: response.AttemptID, Transaction: fromTransaction(response.Transaction), RedirectUrl: response.RedirectURL}
	if response.Challenge != nil {
		result.Challenge = &pb.Challenge{Id: string(response.Challenge.ID), Selection: fromSelection(response.Challenge.Selection), ExpiresAt: timestamppb.New(response.Challenge.ExpiresAt)}
	}

	return result, nil
}
