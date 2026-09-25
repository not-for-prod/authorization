package entity_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/service"
)

func TestProofBindingIsIndependentOfIntent(t *testing.T) {
	for _, intent := range []entity.Intent{entity.IntentSignIn, entity.IntentSignUp} {
		for _, known := range []bool{false, true} {
			t.Run(string(intent)+map[bool]string{false: "/unresolved", true: "/registered"}[known], func(t *testing.T) {
				tx, now, selection := transaction(t)
				tx.Intent, tx.Policy.Intent = intent, intent
				if err := tx.ClaimOperation("start", entity.OperationStart, selection, "", now); err != nil {
					t.Fatal(err)
				}
				if err := tx.RecordMethodStarted("start", nil, "", now); err != nil {
					t.Fatal(err)
				}
				if err := tx.ClaimOperation("verify", entity.OperationVerify, selection, "", now); err != nil {
					t.Fatal(err)
				}
				identity := entity.VerifiedIdentity{Kind: "oauth", Issuer: "google", Subject: "alice"}
				userID := entity.UserID("")
				if known {
					userID = "alice"
				}
				proof := entity.Evidence{Requirement: selection.Requirement, Method: selection.Method, Provider: selection.Provider, VerifiedAt: now}
				decision, err := tx.AcceptMethodProof(t.Context(), service.Evaluator{}, "verify", userID, &identity, proof, now)
				if err != nil {
					t.Fatal(err)
				}
				if !decision.Satisfied || tx.Status != entity.TransactionActive || tx.Principal != nil || tx.VerifiedIdentity == nil || len(tx.Evidence) != 1 {
					t.Fatal("shared acceptance failed or finalized early", tx)
				}
				if (tx.UserID != nil) != known {
					t.Fatal("incorrect user binding")
				}
				identity.Subject = "mutated"
				copy := tx.Copy()
				copy.VerifiedIdentity.Subject = "also mutated"
				if tx.VerifiedIdentity.Subject != "alice" {
					t.Fatal("bound identity aliases input or snapshot")
				}
			})
		}
	}
}

func TestProofBindingSubjectConsistency(t *testing.T) {
	alice := entity.VerifiedIdentity{Kind: "oauth", Issuer: "google", Subject: "alice"}
	linked := entity.VerifiedIdentity{Kind: "oauth", Issuer: "github", Subject: "alice"}
	for _, intent := range []entity.Intent{entity.IntentSignIn, entity.IntentSignUp} {
		for _, tc := range []struct {
			name         string
			boundUser    entity.UserID
			nextUser     entity.UserID
			nextIdentity *entity.VerifiedIdentity
			reject       bool
		}{
			{"same unresolved identity", "", "", &alice, false},
			{"different unresolved identity", "", "", &linked, true},
			{"resolve same identity", "", "alice", &alice, false},
			{"unrelated user-only proof", "", "alice", nil, true},
			{"linked identity same user", "alice", "alice", &linked, false},
			{"different user", "alice", "bob", &linked, true},
			{"unresolved after user binding", "alice", "", &linked, true},
		} {
			t.Run(string(intent)+"/"+tc.name, func(t *testing.T) {
				tx, now, selection := transaction(t)
				tx.Intent, tx.Policy.Intent = intent, intent
				tx.VerifiedIdentity = &alice
				if tc.boundUser != "" {
					value := tc.boundUser
					tx.UserID = &value
				}
				if err := tx.ClaimOperation("start", entity.OperationStart, selection, "", now); err != nil {
					t.Fatal(err)
				}
				if err := tx.RecordMethodStarted("start", nil, "", now); err != nil {
					t.Fatal(err)
				}
				if err := tx.ClaimOperation("verify", entity.OperationVerify, selection, "", now); err != nil {
					t.Fatal(err)
				}
				before, events := tx.Copy(), tx.Events()
				proof := entity.Evidence{Requirement: selection.Requirement, Method: selection.Method, Provider: selection.Provider, VerifiedAt: now}
				_, err := tx.AcceptMethodProof(t.Context(), service.Evaluator{}, "verify", tc.nextUser, tc.nextIdentity, proof, now)
				if tc.reject {
					if !errors.Is(err, entity.ErrSubjectMismatch) {
						t.Fatal("expected subject mismatch", err)
					}
					if !reflect.DeepEqual(before, tx.Copy()) || !reflect.DeepEqual(events, tx.Events()) {
						t.Fatal("rejection mutated transaction")
					}
				} else if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
