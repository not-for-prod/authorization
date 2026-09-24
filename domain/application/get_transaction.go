package application

import (
	"github.com/not-for-prod/proterror/proterror"

	"authorization/domain/entity"
)

// GetTransactionRequest identifies an authentication transaction within its tenant.
type GetTransactionRequest struct {
	TenantID      entity.TenantID
	TransactionID entity.TransactionID
}

// Verify checks request field shape for independent Go application callers.
func (r *GetTransactionRequest) Verify() error {
	if r == nil || r.TenantID == "" || r.TransactionID == "" {
		return &proterror.InvalidArgument{}
	}

	return nil
}

// GetTransactionResponse contains current progress without session credentials.
type GetTransactionResponse struct {
	Transaction TransactionProgress
}
