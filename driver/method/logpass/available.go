package logpass

import (
	"context"

	"authorization/domain/entity"
)

// Available does not inspect identities or perform password hashing.
func (m *Method) Available(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return tx.Intent == entity.IntentSignIn && selection.Method == m.Name() &&
		selection.Provider == "" && selection.Requirement != "", nil
}
