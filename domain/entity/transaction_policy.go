package entity

import (
	"context"
	"slices"
	"time"
)

// PolicyEvaluator keeps the aggregate independent of a concrete evaluation implementation.
type PolicyEvaluator interface {
	Evaluate(context.Context, Transaction, Policy) (Decision, error)
}

// BeginMethodOperation checks the transaction's policy and pending attempt before dispatch.
// False means the same operation already has a stored result and must not be dispatched again.
func (t *Transaction) BeginMethodOperation(ctx context.Context, evaluator PolicyEvaluator, claimID string, kind OperationKind, selection MethodSelection, challengeID ChallengeID, attemptID string, now time.Time) (bool, error) {
	if err := t.CheckUsable(now); err != nil {
		return false, err
	}

	if t.Claim != nil {
		return false, ErrTransactionBusy
	}

	if kind == OperationStart {
		if t.Status != TransactionActive {
			return false, ErrInvalidTransition
		}

		if t.Pending != nil {
			if t.Pending.Ready && t.Pending.Selection == selection {
				return false, nil
			}

			return false, ErrInvalidTransition
		}
	} else {
		if attemptID == "" {
			return false, ErrInvalidTransition
		}

		if t.Status != TransactionActive && (t.Completion == nil || t.Completion.AttemptID != attemptID) {
			return false, ErrInvalidTransition
		}

		for _, receipt := range t.Receipts {
			if receipt.ID == attemptID {
				if receipt.Kind != kind || receipt.Selection != selection || receipt.ChallengeID != challengeID {
					return false, ErrInvalidTransition
				}

				return false, nil
			}
		}

		if err := t.CheckAttempt(attemptID); err != nil {
			return false, err
		}

		if t.Status != TransactionActive || !t.Pending.Ready || t.Pending.Selection != selection || t.Pending.ChallengeID != challengeID {
			return false, ErrInvalidTransition
		}
	}

	decision, err := evaluator.Evaluate(ctx, t.Copy(), t.Policy)
	if err != nil {
		return false, err
	}

	if !slices.Contains(decision.MethodSelections(), selection) {
		return false, ErrInvalidTransition
	}

	if err := t.ClaimOperation(claimID, kind, selection, challengeID, now); err != nil {
		return false, err
	}

	return true, nil
}

// AcceptMethodProof binds the verified subject, accepts evidence, and evaluates policy.
// Finalization belongs to the intent-specific completion step. Failures leave the root unchanged.
func (t *Transaction) AcceptMethodProof(ctx context.Context, evaluator PolicyEvaluator, claimID string, userID UserID, identity *VerifiedIdentity, evidence Evidence, now time.Time) (Decision, error) {
	if identity != nil && (identity.Kind == "" || identity.Issuer == "" || identity.Subject == "") {
		return Decision{}, ErrInvalidProof
	}
	if userID == "" && identity == nil {
		return Decision{}, ErrInvalidProof
	}
	if t.UserID != nil && *t.UserID != userID {
		return Decision{}, ErrSubjectMismatch
	}
	// Unresolved proofs can only accumulate for the same identity. Once resolved,
	// different linked identities are accepted only when they prove the same user.
	if t.UserID == nil && t.VerifiedIdentity != nil && (identity == nil || *t.VerifiedIdentity != *identity) {
		return Decision{}, ErrSubjectMismatch
	}
	next := t.Copy()
	if next.VerifiedIdentity == nil && identity != nil {
		verified := *identity
		next.VerifiedIdentity = &verified
	}
	if err := next.AcceptEvidence(claimID, userID, evidence, now); err != nil {
		return Decision{}, err
	}
	decision, err := evaluator.Evaluate(ctx, next.Copy(), next.Policy)
	if err != nil {
		return Decision{}, err
	}
	next.events = append(t.events, next.events...)
	*t = next
	return decision, nil
}
