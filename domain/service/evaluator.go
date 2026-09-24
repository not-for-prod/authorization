package service

import (
	"context"
	"errors"
	"fmt"

	"authorization/domain/entity"
)

var (
	// ErrInvalidPolicy means the requirement tree cannot be evaluated safely.
	ErrInvalidPolicy = errors.New("invalid authentication policy")
	// ErrPolicyMismatch means the policy belongs to a different tenant or intent.
	ErrPolicyMismatch = errors.New("policy does not match transaction")
)

// MaxRequirementDepth bounds policy nesting, including accidentally cyclic trees.
const MaxRequirementDepth = 64

// Evaluator compares verified transaction evidence with a policy. Its zero value
// is ready to use and may be shared by concurrent callers. It does not verify
// credentials, check transaction expiry, resolve users, or issue sessions.
type Evaluator struct{}

// Evaluate returns an independent, simplified tree of unsatisfied requirements.
// It checks every branch, including alternatives already satisfied by evidence.
// Callers must not mutate the policy or transaction concurrently with this call.
func (Evaluator) Evaluate(ctx context.Context, tx entity.Transaction, policy entity.Policy) (entity.Decision, error) {
	if err := ctx.Err(); err != nil {
		return entity.Decision{}, err
	}

	if policy.TenantID == "" || !policy.Intent.Valid() {
		return entity.Decision{}, fmt.Errorf("%w: tenant and intent are required", ErrInvalidPolicy)
	}

	if tx.TenantID != policy.TenantID || tx.Intent != policy.Intent {
		return entity.Decision{}, ErrPolicyMismatch
	}

	evaluation := policyEvaluation{
		ctx:      ctx,
		verified: make(map[entity.MethodSelection]struct{}, len(tx.Evidence)),
		ids:      make(map[entity.RequirementID]struct{}),
	}

	for _, proof := range tx.Evidence {
		if err := ctx.Err(); err != nil {
			return entity.Decision{}, err
		}

		evaluation.verified[entity.MethodSelection{
			Requirement: proof.Requirement,
			Method:      proof.Method,
			Provider:    proof.Provider,
		}] = struct{}{}
	}

	remaining, err := evaluation.remaining(policy.Root, "", 1)
	if err != nil {
		return entity.Decision{}, err
	}

	return entity.Decision{Satisfied: remaining == nil, Remaining: remaining}, nil
}

type policyEvaluation struct {
	ctx      context.Context
	verified map[entity.MethodSelection]struct{}
	ids      map[entity.RequirementID]struct{}
}

// remaining validates and reduces one node. A method inherits the nearest
// enclosing nonempty requirement ID. Named groups retain their evidence scope;
// anonymous groups with only one remaining child can collapse into that child.
func (e *policyEvaluation) remaining(
	node entity.Requirement,
	scope entity.RequirementID,
	depth int,
) (entity.Requirement, error) {
	if err := e.ctx.Err(); err != nil {
		return nil, err
	}

	if depth > MaxRequirementDepth {
		return nil, fmt.Errorf("%w: requirement nesting exceeds %d", ErrInvalidPolicy, MaxRequirementDepth)
	}

	// Normalize pointers without mutating or retaining the caller's nodes.
	switch typed := node.(type) {
	case *entity.MethodRequirement:
		if typed == nil {
			return nil, fmt.Errorf("%w: nil method requirement", ErrInvalidPolicy)
		}

		node = *typed
	case *entity.AnyOfRequirement:
		if typed == nil {
			return nil, fmt.Errorf("%w: nil any requirement", ErrInvalidPolicy)
		}

		node = *typed
	case *entity.AllOfRequirement:
		if typed == nil {
			return nil, fmt.Errorf("%w: nil all requirement", ErrInvalidPolicy)
		}

		node = *typed
	}

	var (
		id       entity.RequirementID
		children []entity.Requirement
		all      bool
	)

	switch typed := node.(type) {
	case entity.MethodRequirement:
		if scope == "" || typed.Method == "" {
			return nil, fmt.Errorf("%w: method and enclosing requirement ID are required", ErrInvalidPolicy)
		}

		selection := entity.MethodSelection{Requirement: scope, Method: typed.Method}

		_, satisfied := e.verified[selection]
		if len(typed.Providers) != 0 {
			satisfied = false

			for _, provider := range typed.Providers {
				if provider == "" {
					return nil, fmt.Errorf("%w: empty provider in allowlist", ErrInvalidPolicy)
				}

				selection.Provider = provider
				if _, ok := e.verified[selection]; ok {
					satisfied = true
				}
			}
		}

		if satisfied {
			return nil, nil
		}

		return entity.MethodRequirement{
			Method:    typed.Method,
			Providers: append([]entity.ProviderID(nil), typed.Providers...),
		}, nil
	case entity.AnyOfRequirement:
		id, children = typed.ID, typed.Children
	case entity.AllOfRequirement:
		id, children, all = typed.ID, typed.Children, true
	default:
		return nil, fmt.Errorf("%w: missing or unsupported requirement node", ErrInvalidPolicy)
	}

	if len(children) == 0 {
		return nil, fmt.Errorf("%w: requirement groups must have children", ErrInvalidPolicy)
	}

	if id != "" {
		if _, exists := e.ids[id]; exists {
			return nil, fmt.Errorf("%w: duplicate requirement ID", ErrInvalidPolicy)
		}

		e.ids[id] = struct{}{}
		scope = id
	}

	remaining := make([]entity.Requirement, 0, len(children))
	for _, child := range children {
		rest, err := e.remaining(child, scope, depth+1)
		if err != nil {
			return nil, err
		}

		if rest != nil {
			remaining = append(remaining, rest)
		}
	}

	if (all && len(remaining) == 0) || (!all && len(remaining) < len(children)) {
		return nil, nil
	}

	if id == "" && len(remaining) == 1 {
		return remaining[0], nil
	}

	if all {
		return entity.AllOfRequirement{ID: id, Children: remaining}, nil
	}

	return entity.AnyOfRequirement{ID: id, Children: remaining}, nil
}
