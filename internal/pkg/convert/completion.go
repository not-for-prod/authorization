package convert

import (
	"encoding/json"
	"errors"

	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/generated/xo"
)

func CompletionToXO(value entity.CompletionReceipt) (xo.AuthCompletion, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return xo.AuthCompletion{}, errors.Join(&proterror.Internal{}, err)
	}
	return xo.AuthCompletion{
		TenantID: string(value.TenantID),
		ID:       string(value.TransactionID),
		OwnerKey: value.OwnerKey,
		Receipt:  data,
	}, nil
}
func CompletionFromXO(row xo.AuthCompletion) (entity.CompletionReceipt, error) {
	var value entity.CompletionReceipt
	if err := json.Unmarshal(row.Receipt, &value); err != nil {
		return value, errors.Join(&proterror.Internal{}, err)
	}
	if string(value.TenantID) != row.TenantID || string(value.TransactionID) != row.ID ||
		value.OwnerKey != row.OwnerKey ||
		value.Principal.UserID == "" ||
		value.Principal.TenantID != value.TenantID ||
		value.Version == 0 || value.AttemptID == "" ||
		value.ExpiresAt.IsZero() {
		return entity.CompletionReceipt{}, errors.Join(
			&proterror.Internal{},
			errors.New("invalid stored completion receipt"),
		)
	}
	return value, nil
}
