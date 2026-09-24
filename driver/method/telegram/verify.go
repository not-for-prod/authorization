package telegram

import (
	"context"
	"errors"

	"authorization/domain/entity"
	"authorization/domain/gateway"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

// Verify accepts the same authorization-code input as Callback.
func (m *Method) Verify(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
	challenge *entity.Challenge,
	input gateway.MethodInput,
) (gateway.VerificationResult, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if challenge == nil {
		return gateway.VerificationResult{}, span.Err(
			errors.Join(&proterror.Unauthenticated{}, gateway.ErrProofRejected),
		)
	}
	result, err := m.Callback(ctx, tx, selection, *challenge, input)
	if err != nil {
		return gateway.VerificationResult{}, span.Err(err)
	}
	return result, nil
}
