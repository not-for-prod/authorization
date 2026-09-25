package authenticationservice

import (
	"context"
	"crypto/rand"
	"slices"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
)

func (i *Implementation) StartMethod(
	ctx context.Context,
	req application.StartMethodRequest,
) (*application.StartMethodResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if err := req.Verify(); err != nil {
		return nil, span.Err(err)
	}

	caller, err := i.access(ctx, req.TenantID)
	if err != nil {
		return nil, span.Err(err)
	}

	var response application.StartMethodResponse

	err = i.txManager.Do(
		ctx, func(ctx context.Context) error {
			transaction, err := i.transactions.Get(ctx, req.TenantID, req.TransactionID, caller.Key, repository.ForUpdate())
			if err != nil {
				return err
			}

			tenantConfig, ok := i.tenants[req.TenantID]
			if !ok {
				return &proterror.InvalidArgument{}
			}

			method, ok := tenantConfig.methods[req.Selection.Method]
			if !ok {
				return &proterror.InvalidArgument{}
			}

			decision, err := i.evaluator.Evaluate(ctx, transaction.Copy(), transaction.Policy)
			if err != nil {
				return err
			}

			if !slices.Contains(decision.MethodSelections(), req.Selection) {
				return &proterror.FailedPrecondition{}
			}

			available, err := method.Available(ctx, transaction.Copy(), req.Selection)
			if err != nil {
				return err
			}

			if !available {
				return &proterror.FailedPrecondition{}
			}

			attemptID := rand.Text()
			// Existing domain transitions use a claim locally; no lease or retry is persisted.
			if err := transaction.ClaimOperation(
				attemptID,
				entity.OperationStart,
				req.Selection,
				"",
				i.now(),
			); err != nil {
				return err
			}

			startResult, err := method.Start(ctx, transaction.Copy(), req.Selection, req.Input)
			if err != nil {
				return err
			}

			if err := ctx.Err(); err != nil {
				return err
			}

			if err := transaction.RecordMethodStarted(
				attemptID,
				startResult.Challenge,
				startResult.RedirectURL,
				i.now(),
			); err != nil {
				return err
			}

			if err := i.transactions.Update(ctx, *transaction); err != nil {
				return err
			}

			if err := i.events.Publish(ctx, transaction.Events()); err != nil {
				return err
			}

			response = application.StartMethodResponse{
				AttemptID:   attemptID,
				RedirectURL: startResult.RedirectURL,
				Transaction: application.TransactionProgress{
					ID:        transaction.ID,
					TenantID:  transaction.TenantID,
					Intent:    transaction.Intent,
					Status:    transaction.Status,
					CreatedAt: transaction.CreatedAt,
					ExpiresAt: transaction.ExpiresAt,
					PendingAttempt: &application.PendingMethod{
						AttemptID:   attemptID,
						Selection:   req.Selection,
						ChallengeID: transaction.Pending.ChallengeID,
						Ready:       true,
					},
				},
			}

			if c := startResult.Challenge; c != nil {
				response.Challenge = &application.ChallengeReference{ID: c.ID, Selection: c.Selection, ExpiresAt: c.ExpiresAt}
			}

			return nil
		},
	)
	if err != nil {
		return nil, span.Err(err)
	}

	return &response, nil
}
