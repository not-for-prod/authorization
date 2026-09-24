package mock

import (
	"context"

	"authorization"
)

func (m *Method) Available(_ context.Context, tx authorization.Transaction, _ authorization.MethodSelection) (bool, error) {
	return m.id == "password" || tx.UserID != nil, nil
}
