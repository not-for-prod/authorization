package application

import (
	"github.com/not-for-prod/proterror/proterror"

	"authorization/domain/entity"
	"authorization/domain/gateway"
)

// CompleteCallbackRequest identifies the original attempt and decoded callback
// proof. Selection and user binding come from stored state, not callback input.
type CompleteCallbackRequest struct {
	TenantID      entity.TenantID
	TransactionID entity.TransactionID
	// AttemptID is returned by StartMethod and is required even without a challenge.
	AttemptID   string
	ChallengeID entity.ChallengeID
	// Input remains untrusted proof until the callback driver verifies it.
	Input gateway.MethodInput
	// DecodeInput optionally decodes transport input after the application resolves
	// the stored selection. Supply exactly one of Input or DecodeInput. The closure
	// belongs to the delivery adapter and must not perform external side effects.
	DecodeInput func(entity.MethodSelection) (gateway.MethodInput, error)
}

// Verify checks request field shape for independent Go application callers.
func (r *CompleteCallbackRequest) Verify() error {
	if r == nil || r.AttemptID == "" || r.TenantID == "" || r.TransactionID == "" || r.ChallengeID == "" || (r.Input == nil) == (r.DecodeInput == nil) {
		return &proterror.InvalidArgument{}
	}

	return nil
}

// CompleteCallbackResponse reports progress after successful callback acceptance.
type CompleteCallbackResponse struct {
	Transaction TransactionProgress
	// Result is nil while authentication or finalization remains incomplete.
	Result *AuthenticationResult
}
