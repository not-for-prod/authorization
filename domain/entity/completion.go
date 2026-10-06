package entity

import (
	"slices"
	"time"
)

// CompletionReceipt is the immutable, secret-free outcome of finalization.
// It permits recovery without retaining the transaction's policy or challenges.
type CompletionReceipt struct {
	Version       uint64
	TenantID      TenantID
	TransactionID TransactionID
	OwnerKey      string
	AttemptID     string
	ChallengeID   ChallengeID
	Kind          OperationKind
	Intent        Intent
	Principal     Principal
	IssueSession  bool
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

// FreezeCompletion prevents further proof collection while storage finalization retries.
func (t *Transaction) FreezeCompletion(decision Decision, now time.Time) error {
	if err := t.CheckUsable(now); err != nil {
		return err
	}
	if t.Status != TransactionActive || t.Claim != nil || t.Pending != nil ||
		t.Completion == nil || !decision.Satisfied || (t.UserID == nil && t.VerifiedIdentity == nil) {
		return ErrInvalidTransition
	}
	t.Status = TransactionFinalizing
	for i := range t.Challenges {
		t.Challenges[i].State = nil
		if t.Challenges[i].Status == ChallengePending {
			t.Challenges[i].Status = ChallengeRetired
		}
	}
	t.Version++
	return nil
}

func (t Transaction) CheckCompletionOperation(
	attempt string,
	challenge ChallengeID,
	kind OperationKind,
	now time.Time,
) error {
	if err := t.CheckUsable(now); err != nil {
		return err
	}
	if t.Status != TransactionFinalizing && t.Status != TransactionCompleted {
		return ErrInvalidTransition
	}
	if t.Completion == nil || t.Completion.AttemptID != attempt || t.Completion.ChallengeID != challenge ||
		t.Completion.Kind != kind {
		return ErrInvalidTransition
	}
	return nil
}

func (t Transaction) CompletionReceipt() (CompletionReceipt, error) {
	if t.Status != TransactionCompleted || t.Principal == nil || t.Completion == nil {
		return CompletionReceipt{}, ErrInvalidTransition
	}
	principal := *t.Principal
	principal.Evidence = slices.Clone(principal.Evidence)
	return CompletionReceipt{Version: t.Version, TenantID: t.TenantID, TransactionID: t.ID, OwnerKey: t.OwnerKey,
		AttemptID: t.Completion.AttemptID, ChallengeID: t.Completion.ChallengeID, Kind: t.Completion.Kind,
		Intent: t.Intent, Principal: principal, IssueSession: t.Settings.IssueSession,
		CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt}, nil
}

func (r CompletionReceipt) CheckOperation(
	attempt string,
	challenge ChallengeID,
	kind OperationKind,
	now time.Time,
) error {
	if !now.Before(r.ExpiresAt) {
		return ErrTransactionExpired
	}
	if r.AttemptID != attempt || r.ChallengeID != challenge || r.Kind != kind {
		return ErrInvalidTransition
	}
	return nil
}

// RestoreCompletion updates only the optional Redis progress marker; it emits no event.
func (t *Transaction) RestoreCompletion(r CompletionReceipt) error {
	if t.TenantID != r.TenantID || t.ID != r.TransactionID || t.OwnerKey != r.OwnerKey ||
		t.Intent != r.Intent || r.Version < t.Version || !t.ExpiresAt.Equal(r.ExpiresAt) || t.Completion == nil ||
		t.Completion.AttemptID != r.AttemptID || t.Completion.ChallengeID != r.ChallengeID || t.Completion.Kind != r.Kind ||
		(t.Status != TransactionFinalizing && t.Status != TransactionCompleted) {
		return ErrInvalidTransition
	}
	if t.Status == TransactionCompleted {
		return nil
	}
	principal := r.Principal
	principal.Evidence = slices.Clone(principal.Evidence)
	userID := principal.UserID
	t.Principal = &principal
	t.UserID = &userID
	t.VerifiedIdentity = nil
	t.Status = TransactionCompleted
	t.Version = r.Version
	return nil
}
