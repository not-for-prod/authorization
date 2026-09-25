// Package gateway defines contracts implemented by authentication adapters.
package gateway

import (
	"context"
	"errors"

	"github.com/not-for-prod/authorization/domain/entity"
)

// Method verifies one authentication mechanism using dependencies configured by
// the host. The engine selects the tenant's instance before invoking a method.
// Implementations must not mutate the supplied transaction, persist engine
// state, bind users, evaluate policies, or issue sessions.
//
//go:generate moq -out ../../generated/mocks/gateway/method.go -pkg gatewaymock . Method
type Method interface {
	Name() entity.MethodID

	// Available reports whether this selection can currently be used. It must
	// not initiate authentication or send a verification message.
	Available(ctx context.Context, tx entity.Transaction, selection entity.MethodSelection) (bool, error)

	// Start initiates an attempt, optionally producing a challenge and a
	// redirect for the caller. It does not produce authentication evidence.
	Start(
		ctx context.Context,
		tx entity.Transaction,
		selection entity.MethodSelection,
		input MethodInput,
	) (StartResult, error)

	// Verify checks method input against the supplied attempt. Challenge may
	// be nil for direct verification. Results are usable only on nil error.
	Verify(
		ctx context.Context,
		tx entity.Transaction,
		selection entity.MethodSelection,
		challenge *entity.Challenge,
		input MethodInput,
	) (VerificationResult, error)
}

// MethodInput is implemented by a driver's typed input values. The exported
// marker allows driver packages to define inputs without adding provider fields
// to the core. Drivers must reject unexpected concrete types, including nil.
// Inputs may contain credentials and must not be logged or retained as evidence.
type MethodInput interface {
	AuthenticationInput()
}

// StartResult separates private challenge state from a public next action.
// A nil Challenge means no pending challenge is needed. An empty RedirectURL
// means no redirect is requested. Challenge.State must not reach the caller.
// The engine must persist a returned challenge before exposing its redirect.
type StartResult struct {
	Challenge   *entity.Challenge
	RedirectURL string
}

// VerificationResult contains successful proof and the subject it authenticates.
// Exactly one of Identity or UserID must be supplied: Identity for a verified
// identifier to resolve, or UserID when the driver verified an existing user.
// The engine owns resolution, binding, and checking consistency with the
// transaction. Evidence must match the requested requirement/method/provider.
type VerificationResult struct {
	Evidence entity.Evidence
	Identity *VerifiedIdentity
	UserID   *entity.UserID
}

// VerifiedIdentity is a proven issuer/subject pair, not a persisted identity or
// permission to create/link an account. Issuer and Subject must be nonempty.
// Equal reported emails from different issuers do not imply the same user.
type VerifiedIdentity struct {
	Kind    entity.IdentityKind
	Issuer  string
	Subject string
}

// ErrProofRejected is a definitive rejection with no uncertain external outcome.
// Other errors from Start/Verify/Callback leave the dispatch claim in place;
// adapters must sanitize errors because application spans record them.
var ErrProofRejected = errors.New("authentication proof rejected")
