package authenticationservice

import (
	"context"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	pb "github.com/not-for-prod/authorization/generated/pb/authorization/v1"

	"github.com/not-for-prod/observer/tracer/prospan"
)

func (i *Implementation) CreateTransaction(ctx context.Context, req *pb.CreateTransactionRequest) (*pb.CreateTransactionResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if err := i.validate(req); err != nil {
		return nil, span.Err(err)
	}

	var intent entity.Intent
	switch req.Intent {
	case pb.Intent_INTENT_SIGN_UP:
		intent = entity.IntentSignUp
	case pb.Intent_INTENT_SIGN_IN:
		intent = entity.IntentSignIn
	case pb.Intent_INTENT_UPDATE:
		intent = entity.IntentUpdate
	case pb.Intent_INTENT_DELETE:
		intent = entity.IntentDelete
	case pb.Intent_INTENT_UNSPECIFIED:
		// Request validation rejects unspecified and unknown intents.
	}

	response, err := i.app.CreateTransaction(ctx, application.CreateTransactionRequest{TenantID: entity.TenantID(req.TenantId), Intent: intent})
	if err != nil {
		return nil, span.Err(transportError(err))
	}

	return &pb.CreateTransactionResponse{Transaction: fromTransaction(response.Transaction)}, nil
}
