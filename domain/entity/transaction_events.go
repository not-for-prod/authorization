package entity

import (
	"fmt"
	"slices"
	"time"
)

// TransactionEvent describes a successfully completed intent flow, never an intermediate step or proof input.
type TransactionEvent struct {
	ID               string
	Type             string
	SchemaVersion    uint32
	AggregateType    string
	AggregateID      TransactionID
	TenantID         TenantID
	AggregateVersion uint64
	OccurredAt       time.Time
	CorrelationID    string
	CausationID      string
	Source           string
	Before           *TransactionSnapshot
	After            TransactionSnapshot
}

// TransactionSnapshot is the publishable subset of the authentication root.
// Caller keys, private challenge state, redirect URLs, tokens and proof input are omitted.
type TransactionSnapshot struct {
	Status     TransactionStatus
	Version    uint64
	Intent     Intent
	UserID     UserID
	Pending    *AttemptSnapshot
	Claim      *OperationClaim
	Evidence   []Evidence
	Challenges []ChallengeSnapshot
	Attempts   int
	Completion *OperationClaim
	CreatedAt  time.Time
	ExpiresAt  time.Time
}
type ChallengeSnapshot struct {
	ID        ChallengeID
	Selection MethodSelection
	Status    ChallengeStatus
	ExpiresAt time.Time
}

type AttemptSnapshot struct {
	ID          string
	Selection   MethodSelection
	ChallengeID ChallengeID
	Ready       bool
}

func (t Transaction) eventSnapshot() TransactionSnapshot {
	snapshot := TransactionSnapshot{
		Attempts:  t.Attempts,
		Status:    t.Status,
		Version:   t.Version,
		Intent:    t.Intent,
		Evidence:  slices.Clone(t.Evidence),
		CreatedAt: t.CreatedAt,
		ExpiresAt: t.ExpiresAt,
	}
	if t.UserID != nil {
		snapshot.UserID = *t.UserID
	}

	if t.Pending != nil {
		snapshot.Pending = &AttemptSnapshot{
			ID:          t.Pending.ID,
			Selection:   t.Pending.Selection,
			ChallengeID: t.Pending.ChallengeID,
			Ready:       t.Pending.Ready,
		}
	}

	if t.Claim != nil {
		claim := *t.Claim
		snapshot.Claim = &claim
	}

	for _, challenge := range t.Challenges {
		snapshot.Challenges = append(
			snapshot.Challenges,
			ChallengeSnapshot{
				ID:        challenge.ID,
				Selection: challenge.Selection,
				Status:    challenge.Status,
				ExpiresAt: challenge.ExpiresAt,
			},
		)
	}

	if t.Completion != nil {
		completion := *t.Completion
		snapshot.Completion = &completion
	}

	return snapshot
}
func (t *Transaction) recordEvent(kind string, before *TransactionSnapshot, now time.Time) {
	t.Version++

	cause := string(t.ID)
	if t.Claim != nil {
		cause = t.Claim.ID
	} else if before != nil && before.Claim != nil {
		cause = before.Claim.ID
	} else if t.Completion != nil {
		cause = t.Completion.ID
	}

	t.events = append(
		t.events,
		TransactionEvent{
			ID:               fmt.Sprintf("%s/%s/%d", t.TenantID, t.ID, t.Version),
			Type:             kind,
			SchemaVersion:    1,
			AggregateType:    "AuthenticationTransaction",
			AggregateID:      t.ID,
			TenantID:         t.TenantID,
			AggregateVersion: t.Version,
			OccurredAt:       now.UTC(),
			CorrelationID:    string(t.ID),
			CausationID:      cause,
			Source:           "authentication",
			Before:           before,
			After:            t.eventSnapshot(),
		},
	)
}
func (e TransactionEvent) Copy() TransactionEvent {
	if e.Before != nil {
		copy := e.Before.copy()
		e.Before = &copy
	}

	e.After = e.After.copy()

	return e
}
func (s TransactionSnapshot) copy() TransactionSnapshot {
	s.Evidence = slices.Clone(s.Evidence)

	s.Challenges = slices.Clone(s.Challenges)
	if s.Completion != nil {
		completion := *s.Completion
		s.Completion = &completion
	}

	if s.Pending != nil {
		pending := *s.Pending
		s.Pending = &pending
	}

	if s.Claim != nil {
		claim := *s.Claim
		s.Claim = &claim
	}

	return s
}

// Events returns independent unpublished events. Repository snapshots exclude them.
func (t Transaction) Events() []TransactionEvent {
	result := make([]TransactionEvent, len(t.events))
	for index, event := range t.events {
		result[index] = event.Copy()
	}

	return result
}
