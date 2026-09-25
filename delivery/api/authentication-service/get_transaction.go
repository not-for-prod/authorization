package authenticationservice

import (
	"context"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	pb "github.com/not-for-prod/authorization/generated/pb/authorization/v1"

	"github.com/not-for-prod/observer/tracer/prospan"
)

func (i *Implementation) GetTransaction(ctx context.Context, req *pb.GetTransactionRequest) (*pb.GetTransactionResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if err := i.validate(req); err != nil {
		return nil, span.Err(err)
	}

	response, err := i.app.GetTransaction(ctx, application.GetTransactionRequest{TenantID: entity.TenantID(req.TenantId), TransactionID: entity.TransactionID(req.TransactionId)})
	if err != nil {
		return nil, span.Err(transportError(err))
	}

	return &pb.GetTransactionResponse{Transaction: fromTransaction(response.Transaction)}, nil
}
