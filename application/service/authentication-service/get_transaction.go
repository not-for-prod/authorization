package authenticationservice

import (
	"context"
	"errors"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/service"
)

func (i *Implementation) GetTransaction(ctx context.Context, req application.GetTransactionRequest) (*application.GetTransactionResponse, error) {
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

	tx, err := i.transactions.Get(ctx, req.TenantID, req.TransactionID, caller.Key)
	if err != nil {
		return nil, span.Err(err)
	}
	if tx == nil || tx.ID != req.TransactionID || tx.TenantID != req.TenantID {
		return nil, span.Err(&proterror.NotFound{})
	}
	if err := tx.CheckUsable(i.now()); err != nil {
		return nil, span.Err(errors.Join(&proterror.FailedPrecondition{}, err))
	}

	decision, err := (service.Evaluator{}).Evaluate(ctx, *tx, tx.Policy)
	if err != nil {
		if errors.Is(err, service.ErrInvalidPolicy) || errors.Is(err, service.ErrPolicyMismatch) {
			return nil, span.Err(errors.Join(&proterror.FailedPrecondition{}, err))
		}
		return nil, span.Err(err)
	}

	progress := application.TransactionProgress{Status: tx.Status, ID: tx.ID, TenantID: tx.TenantID, Intent: tx.Intent,
		CreatedAt: tx.CreatedAt, ExpiresAt: tx.ExpiresAt, PolicySatisfied: decision.Satisfied}
	if tx.Pending != nil {
		progress.PendingAttempt = &application.PendingMethod{AttemptID: tx.Pending.ID, Selection: tx.Pending.Selection, ChallengeID: tx.Pending.ChallengeID, Ready: tx.Pending.Ready}
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

	return &application.GetTransactionResponse{Transaction: progress}, nil
}
