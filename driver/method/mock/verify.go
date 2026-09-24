package mock

import (
	"context"
	"time"

	"authorization"
)

func (m *Method) Verify(_ context.Context, tx authorization.Transaction, selection authorization.MethodSelection, _ *authorization.Challenge, input authorization.MethodInput) (authorization.VerificationResult, error) {
	proof, ok := input.(Input)
	if !ok || (m.id != "password" && m.id != "totp") {
		return authorization.VerificationResult{}, authorization.ErrProofRejected
	}

	if (m.id == "password" && (proof.Username != "alice" || proof.Password != "test-password")) ||
		(m.id == "totp" && (proof.Code != "123456" || tx.UserID == nil || *tx.UserID != "alice")) {
		return authorization.VerificationResult{}, authorization.ErrProofRejected
	}

	userID := authorization.UserID("alice")

	return authorization.VerificationResult{UserID: &userID, Evidence: authorization.Evidence{
		Requirement: selection.Requirement, Method: selection.Method, Provider: selection.Provider, VerifiedAt: time.Now().UTC(),
	}}, nil
}
