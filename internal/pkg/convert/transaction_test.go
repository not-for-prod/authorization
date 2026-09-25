package convert

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/not-for-prod/authorization/domain/entity"
)

func TestTransactionSnapshotRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	user := entity.UserID("user")
	tx, err := entity.NewTransaction(
		"tx",
		"owner",
		entity.Policy{
			TenantID: "tenant",
			Intent:   entity.IntentSignIn,
			Root: &entity.AllOfRequirement{
				ID: "all",
				Children: []entity.Requirement{
					&entity.AnyOfRequirement{
						ID: "primary",
						Children: []entity.Requirement{
							&entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google"}},
							entity.MethodRequirement{Method: "password"},
						},
					},
				},
			},
		},
		&user,
		entity.TransactionSettings{
			Lifetime:          time.Hour,
			OperationTimeout:  time.Minute,
			MaxAttempts:       5,
			MaxChallenges:     5,
			MaxChallengeBytes: 1024,
			MaxPolicyNodes:    20,
		},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.VerifiedIdentity = &entity.VerifiedIdentity{Issuer: "issuer", Subject: "subject"}
	tx.Pending = &entity.MethodAttempt{ID: "attempt", RedirectURL: "https://provider.test/private"}
	tx.Receipts = []entity.MethodAttempt{{ID: "previous"}}
	tx.Evidence = []entity.Evidence{{Requirement: "primary", Method: "password", VerifiedAt: now}}
	tx.Challenges = []entity.Challenge{{ID: "challenge", State: []byte("private state")}}
	tx.Claim = &entity.OperationClaim{ID: "claim", ExpiresAt: now.Add(time.Minute)}
	tx.Completion = &entity.OperationClaim{ID: "completion", ExpiresAt: now.Add(time.Hour)}
	tx.Principal = &entity.Principal{TenantID: "tenant", UserID: user, Evidence: tx.Evidence, AuthenticatedAt: now}
	row, err := TransactionToXO(*tx)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := TransactionFromXO(row)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tx.Copy(), restored.Copy()) {
		t.Fatalf("snapshot mismatch: %#v != %#v", tx.Copy(), restored.Copy())
	}
	if len(restored.Events()) != 0 {
		t.Fatal("rehydration emitted events")
	}
	restored.Challenges[0].State[0] = 'X'
	if string(tx.Challenges[0].State) != "private state" {
		t.Fatal("shared mutable state")
	}
	var envelope map[string]any
	if err = json.Unmarshal(row.Snapshot, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["version"] = 999
	row.Snapshot, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = TransactionFromXO(row); err == nil {
		t.Fatal("unknown snapshot version accepted")
	}
}
