package authenticationservice

import (
	"context"
	"errors"

	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
)

type completionOperation struct {
	tenant    entity.TenantID
	id        entity.TransactionID
	owner     string
	attempt   string
	challenge entity.ChallengeID
	kind      entity.OperationKind
	user      *entity.UserID
}

// recoverCompletion is shared by VerifyMethod and CompleteCallback. It resolves
// only frozen proof or committed completion, never dispatching a provider call.
func (i *Implementation) recoverCompletion(
	ctx context.Context,
	op completionOperation,
) (*application.VerifyMethodResponse, bool, error) {
	receipt, err := i.completions.Get(ctx, op.tenant, op.id, op.owner)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, true, err
	}
	if errors.Is(err, repository.ErrNotFound) {
		transaction, loadErr := i.transactions.Get(ctx, op.tenant, op.id, op.owner)
		if loadErr != nil {
			return nil, true, loadErr
		}
		if err := checkUpdateCaller(*transaction, application.Caller{Key: op.owner, UserID: op.user}); err != nil {
			return nil, true, err
		}
		if transaction.Status != entity.TransactionFinalizing && transaction.Status != entity.TransactionCompleted {
			return nil, false, nil
		}
		if checkErr := transaction.CheckCompletionOperation(
			op.attempt,
			op.challenge,
			op.kind,
			i.now(),
		); checkErr != nil {
			return nil, true, errors.Join(&proterror.FailedPrecondition{}, checkErr)
		}
		// No Redis or provider calls occur in the SQL scope. The frozen copy cannot
		// acquire a different proof; the receipt key serializes concurrent finalizers.
		err = i.completionManager.Do(ctx, func(ctx context.Context) error {
			var getErr error
			receipt, getErr = i.completions.Get(ctx, op.tenant, op.id, op.owner, repository.ForUpdate())
			if getErr == nil {
				return nil
			}
			if !errors.Is(getErr, repository.ErrNotFound) {
				return getErr
			}
			if checkErr := transaction.CheckCompletionOperation(
				op.attempt,
				op.challenge,
				op.kind,
				i.now(),
			); checkErr != nil {
				return errors.Join(&proterror.FailedPrecondition{}, checkErr)
			}
			decision, evalErr := i.evaluator.Evaluate(ctx, *transaction, transaction.Policy)
			if evalErr != nil {
				return evalErr
			}
			result := &application.AuthenticationResult{}
			if finalizeErr := i.finalizeIntent(
				ctx,
				finalizeIntentRequest{Transaction: transaction, Decision: decision, Result: result},
			); finalizeErr != nil {
				return finalizeErr
			}
			value, receiptErr := transaction.CompletionReceipt()
			if receiptErr != nil {
				return errors.Join(&proterror.FailedPrecondition{}, receiptErr)
			}
			if createErr := i.completions.Create(ctx, value); createErr != nil {
				return createErr
			}
			if publishErr := i.events.Publish(ctx, transaction.Events()); publishErr != nil {
				return publishErr
			}
			receipt = &value
			return nil
		})
		if err != nil {
			return nil, true, err
		}
	}
	if receipt.Intent == entity.IntentUpdate && (op.user == nil || *op.user != receipt.Principal.UserID) {
		return nil, true, &proterror.Unauthenticated{}
	}
	if err = receipt.CheckOperation(op.attempt, op.challenge, op.kind, i.now()); err != nil {
		return nil, true, errors.Join(&proterror.FailedPrecondition{}, err)
	}
	tenant, ok := i.tenants[op.tenant]
	if !ok {
		return nil, true, &proterror.NotFound{}
	}
	if receipt.Intent != entity.IntentDelete {
		user, err := i.users.Get(ctx, receipt.Principal.UserID)
		if err != nil {
			return nil, true, err
		}
		if user == nil || user.ID != receipt.Principal.UserID {
			return nil, true, &proterror.Unauthenticated{}
		}
		if err = i.eligible(ctx, op.tenant, *user); err != nil {
			return nil, true, err
		}
	} else if receipt.IssueSession {
		return nil, true, &proterror.FailedPrecondition{}
	}
	result := &application.AuthenticationResult{Principal: receipt.Principal}
	if receipt.IssueSession {
		if tenant.sessions == nil {
			return nil, true, &proterror.FailedPrecondition{}
		}
		session, sessionErr := tenant.sessions.Create(ctx, gateway.CreateSessionRequest{
			Key: receipt.TransactionID, Principal: receipt.Principal, RecoverUntil: receipt.ExpiresAt,
		})
		if sessionErr != nil {
			return nil, true, sessionErr
		}
		result.Session = &session
	}
	// This marker is a projection of the receipt. Missing progress is harmless;
	// storage failures remain retryable with the same receipt and session key.
	err = i.txManager.Do(ctx, func(ctx context.Context) error {
		transaction, loadErr := i.transactions.Get(ctx, op.tenant, op.id, op.owner, repository.ForUpdate())
		if errors.Is(loadErr, repository.ErrNotFound) {
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		if transaction.Status == entity.TransactionCompleted {
			return nil
		}
		if restoreErr := transaction.RestoreCompletion(*receipt); restoreErr != nil {
			return errors.Join(&proterror.FailedPrecondition{}, restoreErr)
		}
		return i.transactions.Update(ctx, *transaction)
	})
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, true, err
	}
	return &application.VerifyMethodResponse{Result: result, Transaction: application.TransactionProgress{
		ID:              receipt.TransactionID,
		TenantID:        receipt.TenantID,
		Intent:          receipt.Intent,
		Status:          entity.TransactionCompleted,
		CreatedAt:       receipt.CreatedAt,
		ExpiresAt:       receipt.ExpiresAt,
		PolicySatisfied: true,
	}}, true, nil
}
