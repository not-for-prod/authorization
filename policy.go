package authorization

import "github.com/not-for-prod/authorization/domain/entity"

// Policy exposes the entity contract for host implementations.
type Policy = entity.Policy

// Intent exposes the entity contract for host implementations.
type Intent = entity.Intent

const (
	IntentSignUp = entity.IntentSignUp
	IntentSignIn = entity.IntentSignIn
	IntentUpdate = entity.IntentUpdate
	IntentDelete = entity.IntentDelete
)

// RequirementID exposes the entity contract for host implementations.
type RequirementID = entity.RequirementID

// Requirement exposes the entity contract for host implementations.
type Requirement = entity.Requirement

// MethodRequirement exposes the entity contract for host implementations.
type MethodRequirement = entity.MethodRequirement

// AnyOfRequirement exposes the entity contract for host implementations.
type AnyOfRequirement = entity.AnyOfRequirement

// AllOfRequirement exposes the entity contract for host implementations.
type AllOfRequirement = entity.AllOfRequirement

// Decision exposes the entity contract for host implementations.
type Decision = entity.Decision
