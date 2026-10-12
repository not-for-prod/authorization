package authenticationservice

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/service"
)

func (i *Implementation) CreateTransaction(
	ctx context.Context,
	req application.CreateTransactionRequest,
) (*application.CreateTransactionResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if err := req.Verify(); err != nil {
		return nil, span.Err(err)
	}
	caller, err := i.access(ctx, req.TenantID)
	if err != nil {
		return nil, span.Err(err)
	}
	if caller.Key == "" || (caller.UserID != nil && *caller.UserID == "") {
		return nil, span.Err(&proterror.Unauthenticated{})
	}
	if _, ok := i.tenants[req.TenantID]; !ok {
		return nil, span.Err(&proterror.NotFound{})
	}
	rule, ok := i.tenants[req.TenantID].intents[req.Intent]
	if !ok {
		return nil, span.Err(&proterror.FailedPrecondition{})
	}
	if req.AuthenticatedUserID != nil && (caller.UserID == nil || *caller.UserID != *req.AuthenticatedUserID) {
		return nil, span.Err(&proterror.PermissionDenied{})
	}
	if (rule.RequireAuthenticatedUser || req.Intent == entity.IntentDelete || req.Intent == entity.IntentUpdate) && caller.UserID == nil {
		return nil, span.Err(&proterror.Unauthenticated{})
	}
	if req.Intent == entity.IntentSignUp && caller.UserID != nil {
		return nil, span.Err(&proterror.FailedPrecondition{})
	}
	if caller.UserID != nil {
		user, err := i.users.Get(ctx, *caller.UserID)
		if err != nil {
			return nil, span.Err(err)
		}
		if user == nil || user.ID != *caller.UserID {
			return nil, span.Err(&proterror.Unauthenticated{})
		}
		if err := i.eligible(ctx, req.TenantID, *user); err != nil {
			return nil, span.Err(err)
		}
	}
	policy, err := i.policies.Get(ctx, req.TenantID, req.Intent)
	if err != nil {
		return nil, span.Err(err)
	}
	if policy == nil || policy.TenantID != req.TenantID || policy.Intent != req.Intent {
		return nil, span.Err(&proterror.FailedPrecondition{})
	}
	settings := i.limits
	settings.IssueSession = rule.IssueSession
	tx, err := entity.NewTransaction(
		entity.TransactionID(rand.Text()),
		caller.Key,
		*policy,
		caller.UserID,
		settings,
		i.now(),
	)
	if err != nil {
		return nil, span.Err(errors.Join(&proterror.FailedPrecondition{}, err))
	}
	decision, err := (service.Evaluator{}).Evaluate(ctx, *tx, tx.Policy)
	if err != nil {
		if errors.Is(err, service.ErrInvalidPolicy) || errors.Is(err, service.ErrPolicyMismatch) {
			return nil, span.Err(errors.Join(&proterror.FailedPrecondition{}, err))
		}
		return nil, span.Err(err)
	}
	for _, selection := range decision.MethodSelections() {
		if i.tenants[req.TenantID].methods[selection.Method] == nil {
			return nil, span.Err(&proterror.FailedPrecondition{})
		}
	}
	if err := tx.CheckUsable(i.now()); err != nil {
		return nil, span.Err(errors.Join(&proterror.FailedPrecondition{}, err))
	}

	decision, err = (service.Evaluator{}).Evaluate(ctx, *tx, tx.Policy)
	if err != nil {
		if errors.Is(err, service.ErrInvalidPolicy) || errors.Is(err, service.ErrPolicyMismatch) {
			return nil, span.Err(errors.Join(&proterror.FailedPrecondition{}, err))
		}
		return nil, span.Err(err)
	}

	progress := application.TransactionProgress{Status: tx.Status, ID: tx.ID, TenantID: tx.TenantID, Intent: tx.Intent,
		CreatedAt: tx.CreatedAt, ExpiresAt: tx.ExpiresAt, PolicySatisfied: decision.Satisfied}
	if tx.Pending != nil {
		progress.PendingAttempt = &application.PendingMethod{
			AttemptID:   tx.Pending.ID,
			Selection:   tx.Pending.Selection,
			ChallengeID: tx.Pending.ChallengeID,
			Ready:       tx.Pending.Ready,
		}
	} else if tx.Status == entity.TransactionActive && tx.Claim == nil && tx.Attempts < tx.Settings.MaxAttempts {
		for _, selection := range decision.MethodSelections() {
			driver := i.tenants[tx.TenantID].methods[selection.Method]
			if driver == nil {
				return nil, span.Err(&proterror.FailedPrecondition{})
			}

			available, err := driver.Available(ctx, tx.Copy(), selection)
			if err != nil {
				return nil, span.Err(err)
			}

			if available {
				progress.AvailableMethods = append(progress.AvailableMethods, selection)
			}
		}
	}
	if err := i.txManager.Do(ctx, func(ctx context.Context) error {
		if err := i.transactions.Create(ctx, *tx); err != nil {
			return err
		}
		if events := tx.Events(); i.completions == nil || len(events) > 0 {
			return i.events.Publish(ctx, events)
		}
		return nil
	}); err != nil {
		return nil, span.Err(err)
	}
	return &application.CreateTransactionResponse{Transaction: progress}, nil
}
