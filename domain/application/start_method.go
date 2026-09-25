package application

import (
	"time"

	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

// StartMethodRequest selects an eligible method and its decoded driver input.
type StartMethodRequest struct {
	TenantID      entity.TenantID
	TransactionID entity.TransactionID
	Selection     entity.MethodSelection
	// Input is a driver-owned typed value, including for methods with no fields.
	// It may contain credentials and must not be logged or echoed in responses.
	Input gateway.MethodInput
}

// Verify checks request field shape for independent Go application callers.
func (r *StartMethodRequest) Verify() error {
	if r == nil || r.TenantID == "" || r.TransactionID == "" || r.Selection.Requirement == "" || r.Selection.Method == "" || r.Input == nil {
		return &proterror.InvalidArgument{}
	}

	return nil
}

// StartMethodResponse contains progress and any next action from the driver.
type StartMethodResponse struct {
	Transaction TransactionProgress
	AttemptID   string
	// Challenge is nil for methods that do not require a challenge.
	Challenge *ChallengeReference
	// RedirectURL is empty when no browser redirect is needed. Drivers must
	// construct a public authorization URL without server-side secrets.
	RedirectURL string
}

// ChallengeReference identifies a persisted attempt without exposing its state.
type ChallengeReference struct {
	ID        entity.ChallengeID
	Selection entity.MethodSelection
	ExpiresAt time.Time
}
