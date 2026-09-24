package authorization

import (
	"errors"
	"slices"

	"github.com/not-for-prod/proterror/proterror"

	authenticationservice "authorization/application/service/authentication-service"
)

type tenantOptions struct {
	intents  map[Intent]*intentOptions
	sessions SessionManager
}

type intentOptions struct {
	policy    PolicyNode
	hasPolicy bool
	behavior  authenticationservice.IntentConfig
}

// TenantOption configures one tenant's intents and session implementation.
type TenantOption interface{ applyTenant(*tenantOptions) error }
type tenantOptionFunc func(*tenantOptions) error

func (f tenantOptionFunc) applyTenant(c *tenantOptions) error { return f(c) }

// IntentOption configures a policy and completion behavior for one intent.
type IntentOption interface{ applyIntent(*intentOptions) error }
type intentOptionFunc func(*intentOptions) error

func (f intentOptionFunc) applyIntent(c *intentOptions) error { return f(c) }

// WithTenant defines an isolated set of intents and drivers. Duplicate IDs fail.
func WithTenant(id TenantID, opts ...TenantOption) Option {
	opts = slices.Clone(opts)

	return optionFunc(func(c *configuration) error {
		if id == "" || c.tenants[id] != nil {
			return errors.Join(&proterror.InvalidArgument{}, errors.New("tenant ID must be nonempty and unique"))
		}

		tenant := &tenantOptions{intents: make(map[Intent]*intentOptions)}

		for _, opt := range opts {
			if opt == nil {
				return errors.Join(&proterror.InvalidArgument{}, errors.New("nil tenant option"))
			}

			if err := opt.applyTenant(tenant); err != nil {
				return err
			}
		}

		c.tenants[id] = tenant

		return nil
	})
}

// WithSessionManager supplies an explicit issuer, such as a host JWT adapter.
// Configuring it does not enable session issuance for any intent by itself.
func WithSessionManager(manager SessionManager) TenantOption {
	return tenantOptionFunc(func(c *tenantOptions) error {
		if isNil(manager) || c.sessions != nil {
			return errors.Join(&proterror.InvalidArgument{}, errors.New("session manager must be nonnil and configured once"))
		}

		c.sessions = manager

		return nil
	})
}

// WithIntent defines one existing-user authentication flow within a tenant.
func WithIntent(id Intent, opts ...IntentOption) TenantOption {
	opts = slices.Clone(opts)

	return tenantOptionFunc(func(c *tenantOptions) error {
		if !id.Valid() || c.intents[id] != nil {
			return errors.Join(&proterror.InvalidArgument{}, errors.New("intent must be supported and unique within its tenant"))
		}

		intent := &intentOptions{}

		for _, opt := range opts {
			if opt == nil {
				return errors.Join(&proterror.InvalidArgument{}, errors.New("nil intent option"))
			}

			if err := opt.applyIntent(intent); err != nil {
				return err
			}
		}

		c.intents[id] = intent

		return nil
	})
}

// WithPolicy supplies the intent's policy with its concrete method bindings.
func WithPolicy(policy PolicyNode) IntentOption {
	return intentOptionFunc(func(c *intentOptions) error {
		if c.hasPolicy {
			return errors.Join(&proterror.InvalidArgument{}, errors.New("intent policy must be configured once"))
		}

		c.policy, c.hasPolicy = policy, true

		return nil
	})
}

// WithSessionIssuance requires this tenant's explicitly configured SessionManager.
func WithSessionIssuance() IntentOption {
	return intentOptionFunc(func(c *intentOptions) error { c.behavior.IssueSession = true; return nil })
}

// WithAuthenticatedUserRequired requires a user established by the host's Access hook.
func WithAuthenticatedUserRequired() IntentOption {
	return intentOptionFunc(func(c *intentOptions) error { c.behavior.RequireAuthenticatedUser = true; return nil })
}
