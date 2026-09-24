package telegram

import (
	"context"
	"time"

	"authorization/domain/entity"

	"github.com/not-for-prod/observer/tracer/prospan"
)

func (m *Method) Available(
	ctx context.Context,
	tx entity.Transaction,
	selection entity.MethodSelection,
) (bool, error) {
	_, span := prospan.Start(ctx)
	defer span.End()
	if err := ctx.Err(); err != nil {
		return false, span.Err(err)
	}
	return m != nil && m.client != nil && selection.Method == "oauth" && selection.Provider == Provider &&
		selection.Requirement != "" && tx.ID != "" && tx.TenantID != "" &&
		tx.Status == entity.TransactionActive &&
		time.Now().Before(tx.ExpiresAt), nil
}
