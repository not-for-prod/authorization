package application

import (
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

// VerifyMethodRequest supplies proof for a stored method attempt.
type VerifyMethodRequest struct {
	TenantID      entity.TenantID
	TransactionID entity.TransactionID
	// AttemptID is returned by StartMethod and is required even without a challenge.
	AttemptID string
	// ChallengeID is nil for verification without a challenge. Otherwise it must be nonempty
	// and identify an eligible challenge with the same transaction and selection.
	ChallengeID *entity.ChallengeID
	// Input contains decoded driver proof, never caller-asserted evidence.
	Input gateway.MethodInput
	// DecodeInput decodes transport proof using the stored attempt selection.
	// Supply exactly one of Input or DecodeInput; decoding must have no external side effects.
	DecodeInput func(entity.MethodSelection) (gateway.MethodInput, error)
}

// Verify checks request fields for callers entering through the Go application API.
func (r *VerifyMethodRequest) Verify() error {
	if r == nil || r.AttemptID == "" || r.TenantID == "" || r.TransactionID == "" || (r.Input == nil) == (r.DecodeInput == nil) || (r.ChallengeID != nil && *r.ChallengeID == "") {
		return &proterror.InvalidArgument{}
	}

	return nil
}

// GetChallengeID returns the selected challenge, or the zero value for direct verification.
func (r *VerifyMethodRequest) GetChallengeID() entity.ChallengeID {
	if r == nil || r.ChallengeID == nil {
		return ""
	}

	return *r.ChallengeID
}

// VerifyMethodResponse reports progress after successful proof acceptance.
type VerifyMethodResponse struct {
	Transaction TransactionProgress
	// Result is nil while authentication or finalization remains incomplete.
	Result *AuthenticationResult
}
