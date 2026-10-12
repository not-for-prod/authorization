package authenticationservice

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
)

func (i *Implementation) VerifyMethod(
	ctx context.Context,
	req application.VerifyMethodRequest,
) (*application.VerifyMethodResponse, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if err := req.Verify(); err != nil {
		return nil, span.Err(err)
	}

	caller, err := i.access(ctx, req.TenantID)
	if err != nil {
		return nil, span.Err(err)
	}

	var response application.VerifyMethodResponse

	if caller.Key == "" {
		return nil, span.Err(&proterror.PermissionDenied{})
	}
	operation := completionOperation{
		tenant:    req.TenantID,
		id:        req.TransactionID,
		owner:     caller.Key,
		attempt:   req.AttemptID,
		challenge: req.GetChallengeID(),
		kind:      entity.OperationVerify,
		user:      caller.UserID,
	}
	if i.completions != nil {
		recovered, handled, recoverErr := i.recoverCompletion(ctx, operation)
		if recoverErr != nil {
			return nil, span.Err(recoverErr)
		}
		if handled {
			return recovered, nil
		}
	}
	claimID := rand.Text()
	var snapshot entity.Transaction
	var input gateway.MethodInput
	var abandoned bool
	var method gateway.Method
	var selection entity.MethodSelection
	var challenge *entity.Challenge

	err = i.txManager.Do(
		ctx, func(ctx context.Context) error {
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

			if err := checkUpdateCaller(*transaction, caller); err != nil {
				return err
			}
			if err := transaction.CheckUsable(i.now()); err != nil {
				return err
			}
			if err := transaction.CheckAttempt(req.AttemptID); err != nil {
				return err
			}
			if transaction.AbandonExpiredClaim(i.now()) {
				abandoned = true
				if err := i.transactions.Update(ctx, *transaction); err != nil {
					return err
				}
				if events := transaction.Events(); i.completions == nil || len(events) > 0 {
					return i.events.Publish(ctx, events)
				}
				return nil
			}
			if transaction.Intent != entity.IntentSignIn &&
				transaction.Intent != entity.IntentSignUp && transaction.Intent != entity.IntentDelete && transaction.Intent != entity.IntentUpdate {
				return &proterror.Unimplemented{}
			}

			selection = transaction.Pending.Selection

			tenantConfig, ok := i.tenants[req.TenantID]
			if !ok {
				return &proterror.InvalidArgument{}
			}

			method, ok = tenantConfig.methods[selection.Method]
			if !ok {
				return &proterror.InvalidArgument{}
			}

			if err := transaction.ClaimOperation(
				claimID,
				entity.OperationVerify,
				selection,
				req.GetChallengeID(),
				i.now(),
			); err != nil {
				return err
			}

			if req.GetChallengeID() != "" {
				challenge, err = transaction.Challenge(req.GetChallengeID())
				if err != nil {
					return err
				}
			}

			input = req.Input
			if req.DecodeInput != nil {
				input, err = req.DecodeInput(selection)
				if err != nil {
					return err
				}
			}

			if input == nil {
				return &proterror.InvalidArgument{}
			}

			snapshot = transaction.Copy()
			if err := i.transactions.Update(ctx, *transaction); err != nil {
				return err
			}
			if events := transaction.Events(); i.completions == nil || len(events) > 0 {
				return i.events.Publish(ctx, events)
			}
			return nil
		})
	if err != nil {
		return nil, span.Err(err)
	}
	if abandoned {
		return nil, span.Err(
			errors.Join(&proterror.FailedPrecondition{}, entity.ErrTransactionAbandoned),
		)
	}

	verificationResult, err := method.Verify(ctx, snapshot, selection, challenge, input)
	if err != nil {
		if errors.Is(err, gateway.ErrProofRejected) {
			rejectionErr := i.txManager.Do(ctx, func(ctx context.Context) error {
				transaction, loadErr := i.transactions.Get(
					ctx,
					req.TenantID,
					req.TransactionID,
					caller.Key,
					repository.ForUpdate(),
				)
				if loadErr != nil {
					return loadErr
				}
				if rejectErr := transaction.RejectOperation(claimID, false, i.now()); rejectErr != nil {
					return rejectErr
				}
				if saveErr := i.transactions.Update(ctx, *transaction); saveErr != nil {
					return saveErr
				}
				if events := transaction.Events(); i.completions == nil || len(events) > 0 {
					return i.events.Publish(ctx, events)
				}
				return nil
			})
			if rejectionErr != nil {
				return nil, span.Err(rejectionErr)
			}
		}
		return nil, span.Err(err)
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
		if err := transaction.CheckClaim(claimID, i.now()); err != nil {
			return err
		}
		tenantConfig := i.tenants[req.TenantID]

		if err := ctx.Err(); err != nil {
			return err
		}

		userID, identity, err := i.resolveLinkProof(ctx, *transaction, caller, verificationResult)
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

		if decision.Satisfied && i.completions != nil {
			if err := transaction.FreezeCompletion(decision, i.now()); err != nil {
				return errors.Join(&proterror.FailedPrecondition{}, err)
			}
		}
		if decision.Satisfied && i.completions == nil {
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

		if events := transaction.Events(); i.completions == nil || len(events) > 0 {
			if err := i.events.Publish(ctx, events); err != nil {
				return err
			}
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
	},
	)
	if err != nil {
		return nil, span.Err(err)
	}

	if i.completions != nil && response.Transaction.PolicySatisfied {
		recovered, _, recoverErr := i.recoverCompletion(ctx, operation)
		if recoverErr != nil {
			return nil, span.Err(recoverErr)
		}
		return recovered, nil
	}
	return &response, nil
}

type finalizeIntentRequest struct {
	Transaction *entity.Transaction
	Decision    entity.Decision
	Result      *application.AuthenticationResult
}

func (i *Implementation) finalizeIntent(ctx context.Context, req finalizeIntentRequest) error {
	switch req.Transaction.Intent {
	case entity.IntentSignUp:
		if req.Transaction.UserID != nil {
			return errors.Join(&proterror.AlreadyExists{}, repository.ErrAlreadyExists)
		}
		identity := req.Transaction.VerifiedIdentity
		if identity == nil {
			return entity.ErrInvalidProof
		}
		_, err := i.identities.GetBySubject(ctx, identity.Issuer, identity.Subject)
		if err == nil {
			return errors.Join(&proterror.AlreadyExists{}, repository.ErrAlreadyExists)
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		user := entity.User{
			ID:        entity.UserID(rand.Text()),
			Status:    entity.UserStatusActive,
			CreatedAt: i.now().UTC(),
		}
		if err := i.eligible(ctx, req.Transaction.TenantID, user); err != nil {
			return err
		}

		if err := i.users.Create(ctx, user); err != nil {
			return err
		}

		if err := i.identities.Create(
			ctx,
			entity.Identity{
				ID:            entity.IdentityID(rand.Text()),
				UserID:        user.ID,
				Kind:          identity.Kind,
				Issuer:        identity.Issuer,
				Subject:       identity.Subject,
				VerifiedEmail: identity.VerifiedEmail,
				CreatedAt:     user.CreatedAt,
			},
		); err != nil {
			return err
		}

		if err := req.Transaction.BindRegisteredUser(user.ID, req.Decision, i.now()); err != nil {
			return err
		}

	case entity.IntentUpdate:
		if err := i.attachEmail(ctx, req.Transaction); err != nil {
			return err
		}
		if err := req.Transaction.PrepareCompletion(req.Decision, i.now()); err != nil {
			return err
		}

	case entity.IntentDelete:
		if req.Transaction.UserID == nil || req.Transaction.Settings.IssueSession {
			return &proterror.FailedPrecondition{}
		}
		user, err := i.users.Get(ctx, *req.Transaction.UserID, repository.ForUpdate())
		if err != nil {
			return err
		}
		if user == nil || user.ID != *req.Transaction.UserID {
			return entity.ErrInvalidProof
		}
		if err := i.eligible(ctx, req.Transaction.TenantID, *user); err != nil {
			return err
		}
		if err := req.Transaction.PrepareCompletion(req.Decision, i.now()); err != nil {
			return err
		}
		if err := user.Delete(); err != nil {
			return err
		}
		if err := i.users.Update(ctx, *user); err != nil {
			return err
		}

	case entity.IntentSignIn:
		if req.Transaction.UserID == nil {
			return errors.Join(&proterror.NotFound{}, repository.ErrNotFound, application.ErrAccountNotFound)
		}
		user, err := i.users.Get(ctx, *req.Transaction.UserID)
		if err != nil {
			return err
		}
		if user == nil || user.ID != *req.Transaction.UserID {
			return entity.ErrInvalidProof
		}
		if err := i.eligible(ctx, req.Transaction.TenantID, *user); err != nil {
			return err
		}
		if err := i.retainOAuthEmail(ctx, req.Transaction, *user); err != nil {
			return err
		}
		if err := req.Transaction.PrepareCompletion(req.Decision, i.now()); err != nil {
			return err
		}
	}

	principal := *req.Transaction.Principal
	principal.Evidence = slices.Clone(principal.Evidence)
	req.Result.Principal = principal

	if req.Transaction.Settings.IssueSession && i.completions == nil {
		session, err := i.tenants[req.Transaction.TenantID].sessions.Create(
			ctx,
			gateway.CreateSessionRequest{
				Key:          req.Transaction.ID,
				Principal:    principal,
				RecoverUntil: req.Transaction.ExpiresAt,
			},
		)
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		req.Result.Session = &session
	}

	if err := req.Transaction.Complete(i.now()); err != nil {
		return err
	}

	return nil
}
