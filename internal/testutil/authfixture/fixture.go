// Package authfixture supplies shared deterministic authentication test doubles.
package authfixture

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	"github.com/not-for-prod/proterror/proterror"

	authenticationservice "github.com/not-for-prod/authorization/application/service/authentication-service"
	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
	identityinmem "github.com/not-for-prod/authorization/driver/identity/inmem"
	managerinmem "github.com/not-for-prod/authorization/driver/transactionmanager/inmem"
	userinmem "github.com/not-for-prod/authorization/driver/user/inmem"
	gatewaymock "github.com/not-for-prod/authorization/generated/mocks/gateway"
	repositorymock "github.com/not-for-prod/authorization/generated/mocks/repository"
)

type callerKey struct{}

func WithCaller(ctx context.Context, key string, user *entity.UserID) context.Context {
	return context.WithValue(ctx, callerKey{}, application.Caller{Key: key, UserID: user})
}

type Input struct {
	UserID   entity.UserID
	Identity bool
	Reject   bool
}

func (Input) AuthenticationInput() {}

type Fixture struct {
	Config     authenticationservice.Config
	Store      *Store
	Users      *repositorymock.UserMock
	Identities *repositorymock.IdentityMock
	Methods    map[entity.MethodID]*gatewaymock.CallbackMethodMock
	Sessions   *gatewaymock.SessionManagerMock
	Policy     *entity.Policy
	mu         sync.Mutex
	now        time.Time
	issued     map[entity.TransactionID]entity.IssuedSession
}

func New() *Fixture {
	manager := managerinmem.New()
	f := &Fixture{Store: newStore(manager), Methods: make(map[entity.MethodID]*gatewaymock.CallbackMethodMock),
		now: time.Now().UTC(), issued: make(map[entity.TransactionID]entity.IssuedSession)}
	users, identities := userinmem.New(manager), identityinmem.New(manager)
	f.Users = &repositorymock.UserMock{
		GetFunc:    users.Get,
		CreateFunc: users.Create,
		UpdateFunc: users.Update,
		DeleteFunc: users.Delete,
	}
	f.Identities = &repositorymock.IdentityMock{GetFunc: identities.Get, GetBySubjectFunc: identities.GetBySubject,
		CreateFunc: identities.Create, DeleteFunc: identities.Delete}
	if err := manager.Do(context.Background(), func(ctx context.Context) error {
		for _, id := range []entity.UserID{"alice", "bob"} {
			if err := users.Create(
				ctx,
				entity.User{ID: id, Status: entity.UserStatusActive, CreatedAt: f.Now()},
			); err != nil {
				return err
			}
			if err := identities.Create(ctx, entity.Identity{ID: entity.IdentityID(id), UserID: id,
				Kind: "oauth", Issuer: "google", Subject: string(id), CreatedAt: f.Now()}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		panic(err)
	}
	f.Policy = &entity.Policy{
		TenantID: "demo",
		Intent:   entity.IntentSignIn,
		Root: entity.AllOfRequirement{Children: []entity.Requirement{
			entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
				entity.MethodRequirement{
					Method: "password",
				},
				entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google"}},
			}},
			entity.AnyOfRequirement{
				ID:       "mfa",
				Children: []entity.Requirement{entity.MethodRequirement{Method: "totp"}},
			},
		}},
	}
	var methods []gateway.Method
	for _, id := range []entity.MethodID{"password", "oauth", "totp"} {
		method := f.method(id)
		f.Methods[id] = method
		methods = append(methods, method)
	}
	f.Sessions = &gatewaymock.SessionManagerMock{CreateFunc: f.createSession}
	f.Config = authenticationservice.Config{
		Events:             f.Store,
		Transactions:       f.Store,
		TransactionManager: f.Store,
		Users:              f.Users,
		Identities:         f.Identities,
		Now:                f.Now,
		Policies: &repositorymock.PolicyMock{
			GetFunc: func(_ context.Context, tenant entity.TenantID, intent entity.Intent) (*entity.Policy, error) {
				if tenant != f.Policy.TenantID || intent != f.Policy.Intent {
					return nil, repository.ErrNotFound
				}
				policy := *f.Policy
				return &policy, nil
			},
		},
		Access: func(ctx context.Context, tenant entity.TenantID) (application.Caller, error) {
			caller, ok := ctx.Value(callerKey{}).(application.Caller)
			if !ok || caller.Key == "" || tenant != "demo" {
				return application.Caller{}, &proterror.PermissionDenied{}
			}
			return caller, nil
		},
		Eligible: func(_ context.Context, _ entity.TenantID, user entity.User) error {
			if user.Status != entity.UserStatusActive {
				return &proterror.PermissionDenied{}
			}
			return nil
		},
		Tenants: map[entity.TenantID]authenticationservice.TenantConfig{"demo": {
			Methods: methods,
			Intents: map[entity.Intent]authenticationservice.IntentConfig{
				entity.IntentSignIn: {IssueSession: true},
			},
			Sessions: f.Sessions,
		}},
		Limits: entity.TransactionSettings{Lifetime: 5 * time.Minute, OperationTimeout: 10 * time.Second,
			MaxAttempts: 10, MaxChallenges: 10, MaxChallengeBytes: 4096, MaxPolicyNodes: 100},
	}
	return f
}

func (f *Fixture) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fixture) Advance(duration time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(duration)
}

func (f *Fixture) SessionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.issued)
}

func (f *Fixture) createSession(ctx context.Context, req gateway.CreateSessionRequest) (entity.IssuedSession, error) {
	AssertOutsideTransaction(ctx)
	if err := ctx.Err(); err != nil {
		return entity.IssuedSession{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	issued, exists := f.issued[req.Key]
	if !exists {
		issued = entity.IssuedSession{Token: rand.Text(), Session: entity.Session{ID: entity.SessionID(rand.Text()),
			Principal: req.Principal, IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour)}}
		f.issued[req.Key] = issued
	}
	issued.Session.Principal.Evidence = append([]entity.Evidence(nil), issued.Session.Principal.Evidence...)
	return issued, nil
}

func (f *Fixture) method(id entity.MethodID) *gatewaymock.CallbackMethodMock {
	verify := func(ctx context.Context, _ entity.Transaction, selection entity.MethodSelection, _ *entity.Challenge, input gateway.MethodInput) (gateway.VerificationResult, error) {
		AssertOutsideTransaction(ctx)
		proof, ok := input.(Input)
		if !ok || proof.Reject {
			return gateway.VerificationResult{}, gateway.ErrProofRejected
		}
		result := gateway.VerificationResult{Evidence: entity.Evidence{Requirement: selection.Requirement,
			Method: selection.Method, Provider: selection.Provider, VerifiedAt: f.Now()}}
		if proof.Identity {
			result.Identity = &gateway.VerifiedIdentity{Kind: "oauth", Issuer: "google", Subject: string(proof.UserID)}
		} else {
			result.UserID = &proof.UserID
		}
		return result, nil
	}
	return &gatewaymock.CallbackMethodMock{
		NameFunc: func() entity.MethodID { return id },
		AvailableFunc: func(_ context.Context, tx entity.Transaction, _ entity.MethodSelection) (bool, error) {
			return id != "totp" || tx.UserID != nil, nil
		},
		StartFunc: func(_ context.Context, tx entity.Transaction, selection entity.MethodSelection, _ gateway.MethodInput) (gateway.StartResult, error) {
			if id != "oauth" {
				return gateway.StartResult{}, nil
			}
			return gateway.StartResult{RedirectURL: "https://example.com/authorize", Challenge: &entity.Challenge{
				ID: entity.ChallengeID(
					rand.Text(),
				),
				TransactionID: tx.ID,
				Selection:     selection,
				Status:        entity.ChallengePending,
				CreatedAt:     f.Now(),
				ExpiresAt:     f.Now().Add(time.Minute),
				State:         []byte("private-provider-state"),
			}}, nil
		},
		VerifyFunc: verify,
		CallbackFunc: func(ctx context.Context, tx entity.Transaction, selection entity.MethodSelection, challenge entity.Challenge, input gateway.MethodInput) (gateway.VerificationResult, error) {
			return verify(ctx, tx, selection, &challenge, input)
		},
	}
}
