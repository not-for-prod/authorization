package authorization

import "authorization/domain/entity"

// ChallengeID exposes the entity contract for host implementations.
type ChallengeID = entity.ChallengeID

// Challenge exposes the entity contract for host implementations.
type Challenge = entity.Challenge

// ChallengeStatus exposes the entity contract for host implementations.
type ChallengeStatus = entity.ChallengeStatus

// MethodID exposes the entity contract for host implementations.
type MethodID = entity.MethodID

// ProviderID exposes the entity contract for host implementations.
type ProviderID = entity.ProviderID

// MethodSelection exposes the entity contract for host implementations.
type MethodSelection = entity.MethodSelection

// State and operation values retain their domain semantics.
const (
	ChallengePending  = entity.ChallengePending
	ChallengeConsumed = entity.ChallengeConsumed
	ChallengeRetired  = entity.ChallengeRetired
)
