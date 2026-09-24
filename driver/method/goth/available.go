package goth

import (
	"authorization/domain/entity"
	"context"
)

func (m *Method) Available(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return tx.TenantID == m.config.TenantID && selection.Method == m.Name() &&
		selection.Provider == Google &&
		selection.Requirement != "", nil
}
