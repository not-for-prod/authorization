package logpass

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

// StartInput starts a direct password attempt without collecting credentials.
type StartInput struct{}

func (StartInput) AuthenticationInput() {}

// Start creates no private challenge; the engine owns the required pending attempt.
func (m *Method) Start(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
	input gateway.MethodInput,
) (gateway.StartResult, error) {
	available, err := m.Available(ctx, tx, selection)
	if err != nil {
		return gateway.StartResult{}, err
	}
	_, valid := input.(StartInput)
	if !available || !valid {
		return gateway.StartResult{}, gateway.ErrProofRejected
	}
	return gateway.StartResult{}, nil
}
