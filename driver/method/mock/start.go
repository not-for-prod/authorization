package mock

import (
	"context"

	"authorization"
)

func (*Method) Start(context.Context, authorization.Transaction, authorization.MethodSelection, authorization.MethodInput) (authorization.StartResult, error) {
	return authorization.StartResult{}, nil
}
