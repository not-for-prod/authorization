package goth

import (
	"context"
	"errors"

	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

// Verify accepts the same correlated callback proof as Callback, never a raw token.
func (m *Method) Verify(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
	challenge *entity.Challenge,
	input gateway.MethodInput,
) (gateway.VerificationResult, error) {
	if challenge == nil {
		return gateway.VerificationResult{}, errors.Join(
			&proterror.InvalidArgument{},
			gateway.ErrProofRejected,
		)
	}
	return m.Callback(ctx, tx, selection, *challenge, input)
}
