package application

import (
	"context"
	"time"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Authentication defines use cases implemented by the application layer.
// Callers establish trusted tenant and transaction access before invoking it;
// resource IDs alone do not authorize access. Embedded callers must satisfy the
// same trust and input-validation boundaries as delivery adapters.
// Implementations enforce lifecycle, policy, and subject-binding invariants.
// A successful call returns a non-nil response; responses are usable only when
// the error is nil. An error does not imply that a provider had no side effect.
type Authentication interface {
	// GetAuthPolicy describes configured choices without creating a transaction.
	GetAuthPolicy(ctx context.Context, req GetAuthPolicyRequest) (*GetAuthPolicyResponse, error)

	// CreateTransaction starts an authentication transaction for the tenant's configured intent.
	CreateTransaction(ctx context.Context, req CreateTransactionRequest) (*CreateTransactionResponse, error)

	// GetTransaction reads progress without changing state or issuing credentials.
	GetTransaction(ctx context.Context, req GetTransactionRequest) (*GetTransactionResponse, error)

	// StartMethod starts an eligible method and persists any returned challenge
	// before exposing its reference or redirect URL to the caller.
	StartMethod(ctx context.Context, req StartMethodRequest) (*StartMethodResponse, error)

	// VerifyMethod verifies proof and advances the transaction, retaining an unregistered
	// identity during sign-up or resolving the existing user during sign-in.
	VerifyMethod(ctx context.Context, req VerifyMethodRequest) (*VerifyMethodResponse, error)

	// CompleteCallback verifies callback input using the stored challenge's method
	// and applies the same evidence-acceptance rules as VerifyMethod.
	CompleteCallback(ctx context.Context, req CompleteCallbackRequest) (*CompleteCallbackResponse, error)
}

// TransactionProgress describes the currently usable authentication options.
// It excludes private evidence and user binding. AvailableMethods is a flat list
// filtered by the remaining policy and driver availability, not a policy tree.
type PendingMethod struct {
	AttemptID   string
	Selection   entity.MethodSelection
	ChallengeID entity.ChallengeID
	Ready       bool
}

type TransactionProgress struct {
	Status           entity.TransactionStatus
	PendingAttempt   *PendingMethod
	ID               entity.TransactionID
	TenantID         entity.TenantID
	Intent           entity.Intent
	CreatedAt        time.Time
	ExpiresAt        time.Time
	AvailableMethods []entity.MethodSelection
	PolicySatisfied  bool
}

// AuthenticationResult is returned only after subject resolution and authentication transaction
// finalization. Policy satisfaction alone is insufficient. Session is nil when
// the configured intent does not issue a session; otherwise its principal must
// match Principal. Delivery must explicitly map these domain values to public
// responses, excluding evidence and protecting the issued token.
type AuthenticationResult struct {
	Principal entity.Principal
	Session   *entity.IssuedSession
}

// Caller is established by the host for a tenant. Key is a stable, unforgeable
// caller association (for example an authenticated browser-session identifier),
// not a client-selected value. UserID is optional before primary authentication.
type Caller struct {
	Key    string
	UserID *entity.UserID
}

// Access authorizes the tenant and derives the trusted caller from context.
// Hosts use the same association for HTTP, gRPC, embedded calls and callbacks.
type Access func(context.Context, entity.TenantID) (Caller, error)
