package application

import (
	"github.com/not-for-prod/proterror/proterror"

	"authorization/domain/entity"
)

// GetAuthPolicyRequest selects the tenant whose public configuration is requested.
type GetAuthPolicyRequest struct{ TenantID entity.TenantID }

// Verify checks request field shape for independent Go application callers.
func (r *GetAuthPolicyRequest) Verify() error {
	if r == nil || r.TenantID == "" {
		return &proterror.InvalidArgument{}
	}

	return nil
}

// GetAuthPolicyResponse contains independent policy snapshots sorted by intent.
// Authenticated-user-only intents are omitted for anonymous callers. These are
// configured possibilities, not driver availability or transaction progress.
type GetAuthPolicyResponse struct{ Intents []entity.Policy }
