package entity

import (
	"errors"
	"slices"
)

// Policy defines acceptable authentication proofs for a tenant and intent.
type Policy struct {
	TenantID TenantID
	Intent   Intent
	Root     Requirement
}

// Intent identifies a supported account lifecycle flow. Proof requirements remain configurable.
type Intent string

const (
	IntentSignUp Intent = "sign_up"
	IntentSignIn Intent = "sign_in"
	IntentUpdate Intent = "update"
	IntentDelete Intent = "delete"
)

// Valid reports whether the intent is a supported lifecycle flow.
func (i Intent) Valid() bool {
	switch i {
	case IntentSignUp, IntentSignIn, IntentUpdate, IntentDelete:
		return true
	default:
		return false
	}
}

// RequirementID identifies the requirement to which verified evidence belongs.
// Names such as primary and mfa have no built-in semantics.
type RequirementID string

// Requirement is a node in a policy's requirement tree.
// Only the domain's method, any-of, and all-of nodes implement this interface.
type Requirement interface {
	requirement()
}

// MethodRequirement requires proof from a method and, when specified, one of
// its allowed providers.
type MethodRequirement struct {
	Method    MethodID
	Providers []ProviderID
}

func (MethodRequirement) requirement() {}

// AnyOfRequirement is satisfied when at least one child is satisfied.
type AnyOfRequirement struct {
	ID       RequirementID
	Children []Requirement
}

func (AnyOfRequirement) requirement() {}

// AllOfRequirement is satisfied when every child is satisfied.
// The order of its children does not prescribe an execution sequence.
type AllOfRequirement struct {
	ID       RequirementID
	Children []Requirement
}

func (AllOfRequirement) requirement() {}

// Decision describes the result of evaluating transaction evidence against a
// policy. Remaining is the unsatisfied tree, or nil when Satisfied is true.
type Decision struct {
	Satisfied bool
	Remaining Requirement
}

// Snapshot bounds and copies a policy before it becomes persistent transaction state.
func (p Policy) Snapshot(maxNodes int) (Policy, error) {
	remaining := maxNodes

	var visit func(Requirement, int) error

	visit = func(node Requirement, depth int) error {
		remaining--
		if remaining < 0 || depth > 64 {
			return errors.New("authentication policy exceeds configured size")
		}

		switch n := node.(type) {
		case *MethodRequirement:
			if n != nil {
				return visit(*n, depth+1)
			}
		case *AnyOfRequirement:
			if n != nil {
				return visit(*n, depth+1)
			}
		case *AllOfRequirement:
			if n != nil {
				return visit(*n, depth+1)
			}
		case MethodRequirement:
			remaining -= len(n.Providers)
			if remaining >= 0 {
				return nil
			}
		case AnyOfRequirement:
			for _, child := range n.Children {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}

			return nil
		case AllOfRequirement:
			for _, child := range n.Children {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}

			return nil
		}

		return errors.New("invalid authentication policy node")
	}

	if maxNodes <= 0 {
		return Policy{}, errors.New("authentication policy node limit must be positive")
	}

	if err := visit(p.Root, 1); err != nil {
		return Policy{}, err
	}

	p.Root = copyRequirement(p.Root)

	return p, nil
}

func copyRequirement(node Requirement) Requirement {
	switch n := node.(type) {
	case *MethodRequirement:
		if n == nil {
			return nil
		}

		return copyRequirement(*n)
	case *AnyOfRequirement:
		if n == nil {
			return nil
		}

		return copyRequirement(*n)
	case *AllOfRequirement:
		if n == nil {
			return nil
		}

		return copyRequirement(*n)
	case MethodRequirement:
		n.Providers = append([]ProviderID(nil), n.Providers...)
		return n
	case AnyOfRequirement:
		children := make([]Requirement, len(n.Children))
		for i, c := range n.Children {
			children[i] = copyRequirement(c)
		}

		n.Children = children

		return n
	case AllOfRequirement:
		children := make([]Requirement, len(n.Children))
		for i, c := range n.Children {
			children[i] = copyRequirement(c)
		}

		n.Children = children

		return n
	default:
		return nil
	}
}

// MethodSelections lists options in an evaluator-produced remaining tree,
// retaining inherited requirement IDs. It does not check driver availability.
func (d Decision) MethodSelections() []MethodSelection {
	var result []MethodSelection

	var visit func(Requirement, RequirementID)

	visit = func(node Requirement, scope RequirementID) {
		switch n := node.(type) {
		case MethodRequirement:
			providers := n.Providers
			if len(providers) == 0 {
				providers = []ProviderID{""}
			}

			for _, provider := range providers {
				selection := MethodSelection{Requirement: scope, Method: n.Method, Provider: provider}
				if !slices.Contains(result, selection) {
					result = append(result, selection)
				}
			}
		case AnyOfRequirement:
			if n.ID != "" {
				scope = n.ID
			}

			for _, child := range n.Children {
				visit(child, scope)
			}
		case AllOfRequirement:
			if n.ID != "" {
				scope = n.ID
			}

			for _, child := range n.Children {
				visit(child, scope)
			}
		}
	}

	visit(d.Remaining, "")

	return result
}
