package authenticationservice

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"authorization/domain/application"
	"authorization/domain/entity"
	"authorization/domain/gateway"
	"authorization/domain/repository"
)

func (i *Implementation) CompleteCallback(
	ctx context.Context,
	req application.CompleteCallbackRequest,
) (*application.CompleteCallbackResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if err := req.Verify(); err != nil {
		return nil, span.Err(err)
	}

	caller, err := i.access(ctx, req.TenantID)
	if err != nil {
		return nil, span.Err(err)
	}

	var response application.CompleteCallbackResponse

	if caller.Key == "" {
		return nil, span.Err(&proterror.PermissionDenied{})
	}
	transaction, err := i.transactions.Get(
		ctx,
		req.TenantID,
		req.TransactionID,
		caller.Key,
	)
	if err != nil {
		return nil, span.Err(err)
	}

	if err := transaction.CheckUsable(i.now()); err != nil {
		return nil, span.Err(err)
	}
	if err := transaction.CheckAttempt(req.AttemptID); err != nil {
		return nil, span.Err(err)
	}
	if transaction.Intent != entity.IntentSignIn && transaction.Intent != entity.IntentSignUp {
		return nil, span.Err(&proterror.Unimplemented{})
	}

	challenge, err := transaction.Challenge(req.ChallengeID)
	if err != nil {
		return nil, span.Err(err)
	}

	if transaction.Pending.Selection != challenge.Selection ||
		transaction.Pending.ChallengeID != req.ChallengeID {
		return nil, span.Err(entity.ErrInvalidTransition)
	}

	tenantConfig, ok := i.tenants[req.TenantID]
	if !ok {
		return nil, span.Err(&proterror.InvalidArgument{})
	}

	method, ok := tenantConfig.methods[challenge.Selection.Method]
	if !ok {
		return nil, span.Err(&proterror.InvalidArgument{})
	}

	callback, ok := method.(gateway.CallbackMethod)
	if !ok {
		return nil, span.Err(&proterror.FailedPrecondition{})
	}

	input := req.Input
	if req.DecodeInput != nil {
		input, err = req.DecodeInput(challenge.Selection)
		if err != nil {
			return nil, span.Err(err)
		}
	}

	if input == nil {
		return nil, span.Err(&proterror.InvalidArgument{})
	}

	// Validate dispatch guards on a copy without persisting an operation lease.
	check := transaction.Copy()
	if err := check.ClaimOperation(req.AttemptID, entity.OperationCallback, challenge.Selection, req.ChallengeID, i.now()); err != nil {
		return nil, span.Err(err)
	}

	verificationResult, callbackErr := callback.Callback(
		ctx,
		transaction.Copy(),
		transaction.Pending.Selection,
		*challenge,
		input,
	)
	if callbackErr != nil && !errors.Is(callbackErr, gateway.ErrProofRejected) {
		return nil, span.Err(callbackErr)
	}
	if err := ctx.Err(); err != nil {
		return nil, span.Err(err)
	}

	err = i.txManager.Do(ctx, func(ctx context.Context) error {
		transaction, err := i.transactions.Get(
			ctx,
			req.TenantID,
			req.TransactionID,
			caller.Key,
			repository.ForUpdate(),
		)
		if err != nil {
			return err
		}
		if err := transaction.CheckAttempt(req.AttemptID); err != nil {
			return err
		}
		claimID := rand.Text()
		if err := transaction.ClaimOperation(claimID, entity.OperationCallback, challenge.Selection, req.ChallengeID, i.now()); err != nil {
			return err
		}
		if callbackErr != nil {
			if err := transaction.RejectOperation(claimID, false, i.now()); err != nil {
				return err
			}
			if err := i.transactions.Update(ctx, *transaction); err != nil {
				return err
			}
			return i.events.Publish(ctx, transaction.Events())
		}

		tenantConfig := i.tenants[req.TenantID]

		if err := ctx.Err(); err != nil {
			return err
		}

		userID, identity, err := i.resolveVerifiedSubject(ctx, verificationResult)
		if err != nil {
			return err
		}
		decision, err := transaction.AcceptMethodProof(
			ctx,
			i.evaluator,
			claimID,
			userID,
			identity,
			verificationResult.Evidence,
			i.now(),
		)
		if err != nil {
			return err
		}

		if decision.Satisfied {
			response.Result = &application.AuthenticationResult{}
			if err := i.finalizeIntent(ctx, finalizeIntentRequest{
				Transaction: transaction,
				Decision:    decision,
				Result:      response.Result,
			}); err != nil {
				return err
			}
		}

		if err := i.transactions.Update(ctx, *transaction); err != nil {
			return err
		}

		if err := i.events.Publish(ctx, transaction.Events()); err != nil {
			return err
		}

		response.Transaction = application.TransactionProgress{
			ID:              transaction.ID,
			TenantID:        transaction.TenantID,
			Intent:          transaction.Intent,
			Status:          transaction.Status,
			CreatedAt:       transaction.CreatedAt,
			ExpiresAt:       transaction.ExpiresAt,
			PolicySatisfied: decision.Satisfied,
		}

		for _, selection := range decision.MethodSelections() {
			method := tenantConfig.methods[selection.Method]
			if method == nil {
				return &proterror.FailedPrecondition{}
			}

			available, err := method.Available(ctx, transaction.Copy(), selection)
			if err != nil {
				return err
			}

			if available {
				response.Transaction.AvailableMethods = append(
					response.Transaction.AvailableMethods,
					selection,
				)
			}
		}

		return nil
	})
	if err != nil {
		return nil, span.Err(err)
	}

	if callbackErr != nil {
		return nil, span.Err(callbackErr)
	}

	return &response, nil
}
