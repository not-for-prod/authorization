package entity

import "time"

// ChallengeID identifies a method verification attempt.
type ChallengeID string

// Challenge represents an unfinished attempt to verify a selected method.
// State is private method-owned verification data and must not be exposed to
// callers through transport responses or logs.
type Challenge struct {
	ID            ChallengeID
	TransactionID TransactionID
	Selection     MethodSelection
	Status        ChallengeStatus

	CreatedAt time.Time
	ExpiresAt time.Time

	State []byte
}

// ChallengeStatus describes the lifecycle state of a verification attempt.
// Challenge transitions are owned by the authentication transaction.
type ChallengeStatus string

const (
	ChallengePending  ChallengeStatus = "pending"
	ChallengeConsumed ChallengeStatus = "consumed"
	ChallengeRetired  ChallengeStatus = "retired"
)

// MethodID identifies an authentication mechanism, such as password or oauth.
type MethodID string

// ProviderID identifies a provider within a method, such as google within oauth.
// An empty value represents a method without a provider.
type ProviderID string

// MethodSelection identifies the requirement, method, and optional provider
// selected for an authentication attempt.
type MethodSelection struct {
	Requirement RequirementID
	Method      MethodID
	Provider    ProviderID
}
