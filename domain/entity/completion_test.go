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

func TestFrozenCompletionGuardsAndReceipt(t *testing.T) {
	now := time.Now().UTC()
	tx, err := entity.NewTransaction(
		"tx",
		"owner",
		entity.Policy{
			TenantID: "tenant",
			Intent:   entity.IntentSignUp,
			Root: entity.AnyOfRequirement{
				ID:       "primary",
				Children: []entity.Requirement{entity.MethodRequirement{Method: "email"}},
			},
		},
		nil,
		entity.TransactionSettings{
			Lifetime:          time.Minute,
			OperationTimeout:  time.Second,
			MaxAttempts:       5,
			MaxChallenges:     5,
			MaxChallengeBytes: 256,
			MaxPolicyNodes:    8,
		},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	selection := entity.MethodSelection{Requirement: "primary", Method: "email"}
	if err = tx.ClaimOperation("attempt", entity.OperationStart, selection, "", now); err != nil {
		t.Fatal(err)
	}
	if err = tx.RecordMethodStarted("attempt", nil, "", now); err != nil {
		t.Fatal(err)
	}
	if err = tx.ClaimOperation("claim", entity.OperationVerify, selection, "", now); err != nil {
		t.Fatal(err)
	}
	decision, err := tx.AcceptMethodProof(
		context.Background(),
		service.Evaluator{},
		"claim",
		"",
		&entity.VerifiedIdentity{Kind: "email", Issuer: "email", Subject: "a@example.test"},
		entity.Evidence{Requirement: "primary", Method: "email", VerifiedAt: now},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.FreezeCompletion(decision, now); err != nil {
		t.Fatal(err)
	}
	frozen := tx.Copy()
	if err = tx.ClaimOperation("other", entity.OperationStart, selection, "", now); err == nil {
		t.Fatal("frozen proof accepted another attempt")
	}
	if !reflect.DeepEqual(frozen, tx.Copy()) {
		t.Fatal("rejected mutation changed state")
	}
	if err = tx.CheckCompletionOperation("other", "", entity.OperationVerify, now); err == nil {
		t.Fatal("wrong attempt accepted")
	}
	if err = tx.BindRegisteredUser("user", decision, now); err != nil {
		t.Fatal(err)
	}
	if err = tx.Complete(now); err != nil {
		t.Fatal(err)
	}
	receipt, err := tx.CompletionReceipt()
	if err != nil {
		t.Fatal(err)
	}
	if err = receipt.CheckOperation("attempt", "", entity.OperationVerify, now); err != nil {
		t.Fatal(err)
	}
	if err = receipt.CheckOperation(
		"attempt",
		"",
		entity.OperationVerify,
		tx.ExpiresAt,
	); !errors.Is(
		err,
		entity.ErrTransactionExpired,
	) {
		t.Fatal(err)
	}
	if err = frozen.RestoreCompletion(receipt); err != nil {
		t.Fatal(err)
	}
	receipt.Principal.Evidence[0].Method = "changed"
	if frozen.Principal.Evidence[0].Method != "email" {
		t.Fatal("receipt aliases progress")
	}
	if frozen.Version != tx.Version {
		t.Fatal("completion projection lost aggregate version")
	}
	if len(frozen.Events()) != 0 || len(tx.Events()) != 1 {
		t.Fatal("completion marker emitted duplicate event")
	}
}
