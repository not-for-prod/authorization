package redis_test

import (
	"context"
	"testing"

	auth "github.com/not-for-prod/authorization"
)

func TestDeleteCompletion(t *testing.T) {
	for _, callback := range []bool{false, true} {
		name := "verify"
		if callback {
			name = "callback"
		}
		t.Run(name, func(t *testing.T) {
			f := completionSetup(t)
			signup, err := f.engine.VerifyMethod(f.ctx, f.start(t, auth.IntentSignUp))
			if err != nil {
				t.Fatal(err)
			}
			oldID := signup.Result.Principal.UserID
			f.ctx = context.WithValue(f.ctx, deleteUserKey{}, &oldID)
			req := f.start(t, auth.IntentDelete)
			// Keep an unrelated business record keyed by historical user ID.
			if _, err = f.pool.Exec(
				f.ctx,
				"CREATE TABLE retained_client (user_id text PRIMARY KEY, balance bigint)",
			); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(f.ctx, "INSERT INTO retained_client VALUES ($1,12345)", string(oldID)); err != nil {
				t.Fatal(err)
			}
			complete := func() (*auth.AuthenticationResult, error) {
				if callback {
					result, err := f.engine.CompleteCallback(
						f.ctx,
						auth.CompleteCallbackRequest{
							TenantID:      req.TenantID,
							TransactionID: req.TransactionID,
							AttemptID:     req.AttemptID,
							ChallengeID:   *req.ChallengeID,
							Input:         proof{},
						},
					)
					if err != nil {
						return nil, err
					}
					return result.Result, nil
				}
				result, err := f.engine.VerifyMethod(f.ctx, req)
				if err != nil {
					return nil, err
				}
				return result.Result, nil
			}
			f.events.fail.Store(true)
			if _, err = complete(); err == nil {
				t.Fatal("expected rollback")
			}
			f.counts(t, 1, 1)
			var status string
			if err = f.pool.QueryRow(f.ctx, "SELECT status FROM auth_user WHERE id=$1", string(oldID)).
				Scan(&status); err != nil ||
				status != string(auth.UserStatusActive) {
				t.Fatalf("rollback status: %s %v", status, err)
			}
			f.manager.fail.Store(true)
			if _, err = complete(); err == nil {
				t.Fatal("expected lost commit acknowledgement")
			}
			f.counts(t, 1, 2)
			if err = f.pool.QueryRow(f.ctx, "SELECT status FROM auth_user WHERE id=$1", string(oldID)).
				Scan(&status); err != nil ||
				status != string(auth.UserStatusDeleted) {
				t.Fatalf("deleted status: %s %v", status, err)
			}
			result, err := complete()
			if err != nil || result == nil || result.Session != nil || result.Principal.UserID != oldID {
				t.Fatalf("recovery: %#v %v", result, err)
			}
			var balance int
			if err = f.pool.QueryRow(f.ctx, "SELECT balance FROM retained_client WHERE user_id=$1", string(oldID)).
				Scan(&balance); err != nil ||
				balance != 12345 {
				t.Fatalf("business data changed: %d %v", balance, err)
			}
			f.ctx = context.WithValue(f.ctx, deleteUserKey{}, (*auth.UserID)(nil))
			again, err := f.engine.VerifyMethod(f.ctx, f.start(t, auth.IntentSignUp))
			if err == nil || again != nil && again.Result != nil {
				t.Fatalf("deleted identity registered again: %#v %v", again, err)
			}
			if _, err = f.engine.VerifyMethod(f.ctx, f.start(t, auth.IntentSignIn)); err == nil {
				t.Fatal("deleted user signed in")
			}
			if _, err = complete(); err != nil {
				t.Fatal(err)
			}
			f.counts(t, 1, 2)
		})
	}
}

func TestDeleteRequiresCallerAndMatchingProof(t *testing.T) {
	f := completionSetup(t)
	if _, err := f.engine.CreateTransaction(
		f.ctx,
		auth.CreateTransactionRequest{TenantID: f.tenant, Intent: auth.IntentDelete},
	); err == nil {
		t.Fatal("anonymous deletion accepted")
	}
	signup, err := f.engine.VerifyMethod(f.ctx, f.start(t, auth.IntentSignUp))
	if err != nil {
		t.Fatal(err)
	}
	id := signup.Result.Principal.UserID
	f.ctx = context.WithValue(f.ctx, deleteUserKey{}, &id)
	req := f.start(t, auth.IntentDelete)
	f.method.subject = "not-owned@example.test"
	if _, err = f.engine.VerifyMethod(f.ctx, req); err == nil {
		t.Fatal("unowned proof accepted")
	}
	f.counts(t, 1, 1)
}

func TestDeleteRejectsAnotherRegisteredUser(t *testing.T) {
	f := completionSetup(t)
	first, err := f.engine.VerifyMethod(f.ctx, f.start(t, auth.IntentSignUp))
	if err != nil {
		t.Fatal(err)
	}
	f.method.subject = "another@example.test"
	if _, err = f.engine.VerifyMethod(f.ctx, f.start(t, auth.IntentSignUp)); err != nil {
		t.Fatal(err)
	}
	id := first.Result.Principal.UserID
	f.ctx = context.WithValue(f.ctx, deleteUserKey{}, &id)
	if _, err = f.engine.VerifyMethod(f.ctx, f.start(t, auth.IntentDelete)); err == nil {
		t.Fatal("another registered user's proof accepted")
	}
	f.counts(t, 2, 2)
}

func TestOutstandingSignInCannotRestoreDeletedUser(t *testing.T) {
	f := completionSetup(t)
	signup, err := f.engine.VerifyMethod(f.ctx, f.start(t, auth.IntentSignUp))
	if err != nil {
		t.Fatal(err)
	}
	signin := f.start(t, auth.IntentSignIn)
	id := signup.Result.Principal.UserID
	f.ctx = context.WithValue(f.ctx, deleteUserKey{}, &id)
	if _, err = f.engine.VerifyMethod(f.ctx, f.start(t, auth.IntentDelete)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.engine.VerifyMethod(f.ctx, signin); err == nil {
		t.Fatal("outstanding signin restored deleted account")
	}
	f.counts(t, 1, 2)
}
