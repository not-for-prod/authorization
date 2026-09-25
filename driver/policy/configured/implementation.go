// Package configuredpolicy stores immutable constructor-supplied policies.
package configuredpolicy

import (
	"errors"

	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
)

var _ repository.Policy = (*Implementation)(nil)

type key struct {
	tenant entity.TenantID
	intent entity.Intent
}

type Implementation struct {
	policies map[key]entity.Policy
	maxNodes int
}

// NewImplementation copies policies; no caller-owned map or tree is retained.
func NewImplementation(policies []entity.Policy, maxNodes int) (*Implementation, error) {
	result := &Implementation{policies: make(map[key]entity.Policy, len(policies)), maxNodes: maxNodes}
	for _, policy := range policies {
		snapshot, err := policy.Snapshot(maxNodes)
		if err != nil {
			return nil, errors.Join(&proterror.InvalidArgument{}, err)
		}

		id := key{tenant: policy.TenantID, intent: policy.Intent}
		if _, exists := result.policies[id]; exists {
			return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("duplicate configured policy"))
		}

		result.policies[id] = snapshot
	}

	return result, nil
}
