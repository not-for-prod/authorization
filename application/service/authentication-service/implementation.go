package authenticationservice

import (
	"context"
	"errors"
	"time"

	"authorization/domain/service"

	"github.com/not-for-prod/proterror/proterror"

	"authorization/domain/application"
	"authorization/domain/entity"
	"authorization/domain/gateway"
	"authorization/domain/repository"
)

var _ application.Authentication = (*Implementation)(nil)

// IntentConfig configures policy access and optional session issuance.
// SIGN_UP creates a new user from verified identity proof after policy satisfaction.
type IntentConfig struct {
	RequireAuthenticatedUser bool
	IssueSession             bool
}

type TenantConfig struct {
	Methods  []gateway.Method
	Intents  map[entity.Intent]IntentConfig
	Sessions gateway.SessionManager
}

// Config is constructed by the embedding host. Access and Eligible are required
// authorization boundaries. Hooks/adapters must be concurrency-safe and return
// errors without credentials or raw provider input.
type Config struct {
	Events             repository.EventPublisher
	Transactions       repository.Transaction
	TransactionManager repository.TransactionManager
	Users              repository.User
	Identities         repository.Identity
	Policies           repository.Policy
	Access             application.Access
	Eligible           func(context.Context, entity.TenantID, entity.User) error
	Tenants            map[entity.TenantID]TenantConfig
	Limits             entity.TransactionSettings
	Now                func() time.Time
}

type tenantConfig struct {
	methods  map[entity.MethodID]gateway.Method
	intents  map[entity.Intent]IntentConfig
	sessions gateway.SessionManager
}

type Implementation struct {
	events       repository.EventPublisher
	transactions repository.Transaction
	txManager    repository.TransactionManager
	users        repository.User
	identities   repository.Identity
	policies     repository.Policy
	evaluator    service.Evaluator
	access       application.Access
	eligible     func(context.Context, entity.TenantID, entity.User) error
	tenants      map[entity.TenantID]tenantConfig
	limits       entity.TransactionSettings
	now          func() time.Time
}

func NewImplementation(cfg Config) (*Implementation, error) {
	if cfg.Events == nil || cfg.Transactions == nil || cfg.TransactionManager == nil || cfg.Users == nil || cfg.Identities == nil ||
		cfg.Policies == nil || cfg.Access == nil || cfg.Eligible == nil || len(cfg.Tenants) == 0 {
		return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("authentication dependencies are required"))
	}

	if err := cfg.Limits.Validate(); err != nil {
		return nil, errors.Join(&proterror.InvalidArgument{}, err)
	}

	i := &Implementation{events: cfg.Events, transactions: cfg.Transactions, txManager: cfg.TransactionManager, users: cfg.Users,
		identities: cfg.Identities, policies: cfg.Policies, access: cfg.Access, eligible: cfg.Eligible,
		tenants: make(map[entity.TenantID]tenantConfig), limits: cfg.Limits, now: cfg.Now}
	if i.now == nil {
		i.now = time.Now
	}

	for id, source := range cfg.Tenants {
		if id == "" || len(source.Intents) == 0 || len(source.Methods) == 0 {
			return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("tenant needs methods and intents"))
		}

		tenant := tenantConfig{methods: make(map[entity.MethodID]gateway.Method), intents: make(map[entity.Intent]IntentConfig), sessions: source.Sessions}
		for _, method := range source.Methods {
			if method == nil || method.Name() == "" {
				return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("method name is required"))
			}

			if _, exists := tenant.methods[method.Name()]; exists {
				return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("duplicate method"))
			}

			tenant.methods[method.Name()] = method
		}

		for intent, rule := range source.Intents {
			if !intent.Valid() || (rule.IssueSession && source.Sessions == nil) {
				return nil, errors.Join(
					&proterror.InvalidArgument{},
					errors.New("intent needs a name and configured session issuer"),
				)
			}

			tenant.intents[intent] = rule
		}

		i.tenants[id] = tenant
	}

	return i, nil
}
