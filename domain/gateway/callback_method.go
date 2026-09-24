package gateway

import (
	"context"

	"authorization/domain/entity"
)

// CallbackMethod is an optional verification entry point for methods completed
// through a provider callback. Password and code methods need only implement
// Method. The host parses its transport into driver-owned typed MethodInput.
// Implementations do not register routes or write HTTP responses.
//
//go:generate moq -out ../../generated/mocks/gateway/callback_method.go -pkg gatewaymock . CallbackMethod
type CallbackMethod interface {
	Method

	// Callback verifies provider-returned input against the original challenge,
	// including the provider protocol's correlation and proof checks. The
	// challenge is mandatory; callbacks do not establish their own tenant,
	// transaction, selection, or user binding from untrusted input.
	// Results have the same contract as Verify and are usable only on nil error.
	Callback(
		ctx context.Context,
		tx entity.Transaction,
		selection entity.MethodSelection,
		challenge entity.Challenge,
		input MethodInput,
	) (VerificationResult, error)
}
