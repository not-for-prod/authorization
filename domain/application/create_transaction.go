package application

import (
	"github.com/not-for-prod/proterror/proterror"

	"authorization/domain/entity"
)

// CreateTransactionRequest selects the tenant and intent for a new authentication transaction.
type CreateTransactionRequest struct {
	TenantID entity.TenantID
	Intent   entity.Intent

	// AuthenticatedUserID is supplied only by trusted host authentication when
	// starting an authentication transaction for an existing user. It must never come directly from
	// client input. Nil means that no existing subject has been authenticated.
	AuthenticatedUserID *entity.UserID
}

// Verify checks request field shape for independent Go application callers.
func (r *CreateTransactionRequest) Verify() error {
	if r == nil || r.TenantID == "" || !r.Intent.Valid() || (r.AuthenticatedUserID != nil && *r.AuthenticatedUserID == "") {
		return &proterror.InvalidArgument{}
	}

	return nil
}

// CreateTransactionResponse contains initial progress without session credentials.
type CreateTransactionResponse struct {
	Transaction TransactionProgress
}
