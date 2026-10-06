package authorization

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/not-for-prod/proterror/proterror"

	authenticationservice "github.com/not-for-prod/authorization/application/service/authentication-service"
	"github.com/not-for-prod/authorization/domain/service"
	configuredpolicy "github.com/not-for-prod/authorization/driver/policy/configured"
)

// Dependencies are host-owned ports. Callbacks and adapters must be safe for
// concurrent use. Policies and method bindings are supplied through WithTenant.
type Dependencies struct {
	// Completions and CompletionManager opt into staged cross-store finalization.
	// Both must be supplied; Users, Identities and Events share CompletionManager.
	Completions       CompletionRepository
	CompletionManager TransactionManager
	// Events shares CompletionManager when configured, otherwise TransactionManager.
	Events EventPublisher
	// Transactions persists the entire authentication transaction, including its
	// policy snapshot, challenges, evidence, and completion state. It must use
	// the storage transaction established by TransactionManager.
	Transactions TransactionRepository
	// TransactionManager provides atomic commit/rollback and holds root locks
	// until completion. It must coordinate the same storage as Transactions;
	// participating operations use the context supplied to its Do callback. SIGN_UP
	// additionally requires Users, Identities and Events to share this atomic boundary
	// unless Completions and CompletionManager select staged finalization.
	TransactionManager TransactionManager
	// Users loads existing canonical users after proof verification and before
	// completion. For SIGN_UP it also creates the user in the shared storage transaction.
	Users UserRepository
	// Identities resolves a driver's verified issuer/subject pair to an existing
	// user. Drivers returning a verified UserID bypass this lookup. The engine
	// creates the initial identity during SIGN_UP. It never merges accounts.
	Identities IdentityRepository
	// Access authorizes the requested tenant and derives Caller from trusted host
	// context on every operation. Caller.Key must remain stable across the flow;
	// Caller.UserID is optional until a user has been authenticated by the host.
	Access Access
	// Eligible decides whether an identified user may authenticate in the tenant,
	// for example by checking account status and tenant membership. It runs before
	// user binding and completion. Return nil to allow or a classified error to deny.
	Eligible func(context.Context, TenantID, User) error
}

type configuration struct {
	tenants map[TenantID]*tenantOptions
	limits  TransactionSettings
	now     func() time.Time
}

// Option configures the service. TenantOption and IntentOption cannot be used here.
type Option interface{ apply(*configuration) error }
type optionFunc func(*configuration) error

func (f optionFunc) apply(c *configuration) error { return f(c) }

// WithLimits sets required explicit transaction bounds. IssueSession is controlled
// per intent with WithSessionIssuance, not by this field.
func WithLimits(limits TransactionSettings) Option {
	return optionFunc(func(c *configuration) error { c.limits = limits; return nil })
}

// WithClock supplies a concurrency-safe clock, for example in deterministic tests.
// Without this option the engine uses time.Now.
func WithClock(now func() time.Time) Option {
	return optionFunc(func(c *configuration) error {
		if now == nil {
			return errors.Join(&proterror.InvalidArgument{}, errors.New("clock is required"))
		}

		c.now = now

		return nil
	})
}

// New compiles policy-bound drivers and wires authentication and sign-up flows.
// It performs no storage or provider I/O; driver Name must be stable and pure.
// No session implementation is selected by default. Limits must be explicit.
func New(deps Dependencies, opts ...Option) (Authentication, error) {
	for _, dependency := range []any{deps.Events, deps.Transactions, deps.TransactionManager, deps.Users, deps.Identities, deps.Access, deps.Eligible} {
		if isNil(dependency) {
			return nil, errors.Join(
				&proterror.InvalidArgument{},
				errors.New("authentication dependencies are required"),
			)
		}
	}

	if (deps.Completions != nil && isNil(deps.Completions)) ||
		(deps.CompletionManager != nil && isNil(deps.CompletionManager)) ||
		isNil(deps.Completions) != isNil(deps.CompletionManager) {
		return nil, errors.Join(
			&proterror.InvalidArgument{},
			errors.New("completion storage and manager must be supplied together"),
		)
	}

	cfg := configuration{tenants: make(map[TenantID]*tenantOptions)}

	for _, opt := range opts {
		if opt == nil {
			return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("nil service option"))
		}

		if err := opt.apply(&cfg); err != nil {
			return nil, err
		}
	}

	if err := cfg.limits.Validate(); err != nil {
		return nil, errors.Join(&proterror.InvalidArgument{}, err)
	}

	tenants := make(map[TenantID]authenticationservice.TenantConfig, len(cfg.tenants))

	var policies []Policy

	for id, source := range cfg.tenants {
		tenant := authenticationservice.TenantConfig{
			Intents:  make(map[Intent]authenticationservice.IntentConfig),
			Sessions: source.sessions,
		}
		bindings := make(map[MethodID]*methodBinding)

		for intent, definition := range source.intents {
			if !definition.hasPolicy {
				return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("intent requires a policy"))
			}

			remaining := cfg.limits.MaxPolicyNodes

			root, err := definition.policy.compile(bindings, &remaining, 1)
			if err != nil {
				return nil, err
			}

			policy := Policy{TenantID: id, Intent: intent, Root: root}
			if _, err := (service.Evaluator{}).Evaluate(
				context.Background(),
				Transaction{TenantID: id, Intent: intent},
				policy,
			); err != nil {
				return nil, errors.Join(&proterror.InvalidArgument{}, err)
			}

			policies = append(policies, policy)
			tenant.Intents[intent] = definition.behavior
		}

		for _, binding := range bindings {
			tenant.Methods = append(tenant.Methods, binding.driver)
		}

		tenants[id] = tenant
	}

	policyRepository, err := configuredpolicy.NewImplementation(policies, cfg.limits.MaxPolicyNodes)
	if err != nil {
		return nil, err
	}

	implementation, err := authenticationservice.NewImplementation(authenticationservice.Config{
		Completions: deps.Completions, CompletionManager: deps.CompletionManager,
		Events: deps.Events, Transactions: deps.Transactions, TransactionManager: deps.TransactionManager,
		Users: deps.Users, Identities: deps.Identities, Access: deps.Access, Eligible: deps.Eligible,
		Policies: policyRepository, Tenants: tenants, Limits: cfg.limits, Now: cfg.now,
	})
	if err != nil {
		return nil, err
	}

	return implementation, nil
}

// Interface-valued dependencies and drivers must reject typed nils as well.
func isNil(value any) bool {
	if value == nil {
		return true
	}

	switch v := reflect.ValueOf(value); v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
