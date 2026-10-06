package entity_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/service"
)

type failedEvaluation struct{}

func (failedEvaluation) Evaluate(context.Context, entity.Transaction, entity.Policy) (entity.Decision, error) {
	return entity.Decision{}, errors.New("evaluation failed")
}

func TestPolicyFailureDoesNotAcceptProofOrEmitEvent(t *testing.T) {
	tx, now, selection := transaction(t)
	if _, err := tx.BeginMethodOperation(t.Context(), service.Evaluator{}, "start", entity.OperationStart, selection, "", "", now); err != nil {
		t.Fatal(err)
	}

	if err := tx.RecordMethodStarted("start", nil, "", now); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.BeginMethodOperation(t.Context(), service.Evaluator{}, "verify", entity.OperationVerify, selection, "", "start", now); err != nil {
		t.Fatal(err)
	}

	before := tx.Copy()
	events := tx.Events()

	evidence := entity.Evidence{
		Requirement: selection.Requirement,
		Method:      selection.Method,
		Provider:    selection.Provider,
		VerifiedAt:  now,
	}
	if _, err := tx.AcceptMethodProof(t.Context(), failedEvaluation{}, "verify", "alice", nil, evidence, now); err == nil {
		t.Fatal("evaluation failure accepted")
	}

	if !reflect.DeepEqual(before, tx.Copy()) || !reflect.DeepEqual(events, tx.Events()) {
		t.Fatal("failed transition changed state or events")
	}

	if _, err := tx.AcceptMethodProof(t.Context(), service.Evaluator{}, "verify", "alice", nil, evidence, now); err != nil {
		t.Fatal(err)
	}

	if tx.Status != entity.TransactionActive || tx.Pending != nil || tx.Principal != nil {
		t.Fatal("proof acceptance finalized authentication")
	}
}

func TestEventSnapshotsAreIndependentAndExcludePrivateState(t *testing.T) {
	tx, now, selection := transaction(t)
	if err := tx.ClaimOperation("start", entity.OperationStart, selection, "", now); err != nil {
		t.Fatal(err)
	}

	challenge := &entity.Challenge{
		ID:            "challenge",
		TransactionID: tx.ID,
		Selection:     selection,
		Status:        entity.ChallengePending,
		CreatedAt:     now,
		ExpiresAt:     now.Add(time.Second),
		State:         []byte("secret"),
	}
	if err := tx.RecordMethodStarted("start", challenge, "https://provider.example/authorize", now); err != nil {
		t.Fatal(err)
	}

	if err := tx.ClaimOperation("verify", entity.OperationVerify, selection, challenge.ID, now); err != nil {
		t.Fatal(err)
	}
	proof := entity.Evidence{
		Requirement: selection.Requirement,
		Method:      selection.Method,
		Provider:    selection.Provider,
		VerifiedAt:  now,
	}
	if err := tx.AcceptEvidence("verify", "alice", proof, now); err != nil {
		t.Fatal(err)
	}
	if err := tx.PrepareCompletion(entity.Decision{Satisfied: true}, now); err != nil {
		t.Fatal(err)
	}
	if len(tx.Events()) != 0 {
		t.Fatal("intermediate transition emitted an event")
	}
	if err := tx.Complete(now); err != nil {
		t.Fatal(err)
	}
	original := tx.Events()
	if len(original) != 1 {
		t.Fatalf("expected one completion event: %+v", original)
	}
	if err := tx.Complete(now); err != nil {
		t.Fatal(err)
	}
	copy := tx.Events()
	copy[0].Before.Completion.ID = "mutated"
	copy[0].After.Completion.ID = "mutated"
	copy[0].Before.Evidence[0].Method = "mutated"
	copy[0].After.Challenges[0].ID = "mutated"

	if !reflect.DeepEqual(original, tx.Events()) {
		t.Fatal("event snapshots alias")
	}

	tx.Completion.ID = "later-state"

	tx.Challenges[0].Status = entity.ChallengeRetired
	if !reflect.DeepEqual(original, tx.Events()) {
		t.Fatal("events changed with aggregate")
	}

	if len(tx.Copy().Events()) != 0 {
		t.Fatal("rehydration retained unpublished events")
	}
}
