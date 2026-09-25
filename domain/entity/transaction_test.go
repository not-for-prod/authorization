package entity_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/not-for-prod/authorization/domain/entity"
)

func transaction(t *testing.T) (*entity.Transaction, time.Time, entity.MethodSelection) {
	t.Helper()

	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	selection := entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}
	policy := entity.Policy{TenantID: "tenant", Intent: "sign_in", Root: entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
		entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google"}},
	}}}

	tx, err := entity.NewTransaction("tx", "caller", policy, nil, entity.TransactionSettings{
		Lifetime: time.Minute, OperationTimeout: 10 * time.Second, MaxAttempts: 4, MaxChallenges: 2, MaxChallengeBytes: 32, MaxPolicyNodes: 10,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	return tx, now, selection
}

func TestChallengeConsumptionAndCompletion(t *testing.T) {
	tx, now, selection := transaction(t)
	if err := tx.ClaimOperation("start", entity.OperationStart, selection, "", now); err != nil {
		t.Fatal(err)
	}

	challenge := entity.Challenge{ID: "challenge", TransactionID: tx.ID, Selection: selection, Status: entity.ChallengePending,
		CreatedAt: now, ExpiresAt: now.Add(30 * time.Second), State: []byte("private")}
	if err := tx.AttachChallenge("start", &challenge, now); err != nil {
		t.Fatal(err)
	}

	challenge.State[0] = 'X'
	if string(tx.Challenges[0].State) != "private" {
		t.Fatal("challenge state aliases driver")
	}

	if err := tx.ClaimOperation("verify", entity.OperationCallback, selection, challenge.ID, now); err != nil {
		t.Fatal(err)
	}

	proof := entity.Evidence{Requirement: selection.Requirement, Method: selection.Method, Provider: selection.Provider, VerifiedAt: now}
	if err := tx.AcceptEvidence("verify", "user", proof, now); err != nil {
		t.Fatal(err)
	}

	if tx.Claim != nil || tx.UserID == nil || *tx.UserID != "user" || len(tx.Evidence) != 1 ||
		tx.Challenges[0].Status != entity.ChallengeConsumed || len(tx.Challenges[0].State) != 0 {
		t.Fatal("proof not atomically accepted")
	}

	if err := tx.ClaimOperation("replay", entity.OperationCallback, selection, challenge.ID, now); !errors.Is(err, entity.ErrInvalidTransition) {
		t.Fatalf("replay: %v", err)
	}

	if err := tx.PrepareCompletion(entity.Decision{Satisfied: true}, now); err != nil {
		t.Fatal(err)
	}

	if tx.Status != entity.TransactionFinalizing || tx.Principal.UserID != "user" {
		t.Fatal("missing completion intent")
	}

	if err := tx.Complete(now); err != nil {
		t.Fatal(err)
	}

	if err := tx.Complete(now); err != nil {
		t.Fatal("completion must be idempotent", err)
	}

	copy := tx.Copy()
	copy.Principal.Evidence[0].Method = "other"
	copy.Evidence[0].Method = "other"

	*copy.UserID = "other"
	if tx.Principal.Evidence[0].Method != "oauth" || tx.Evidence[0].Method != "oauth" || *tx.UserID != "user" {
		t.Fatal("copy aliases root")
	}
}

func TestRejectedEvidenceDoesNotMutate(t *testing.T) {
	for _, name := range []string{"wrong claim", "wrong subject", "wrong selection", "future proof", "stale claim", "expired transaction"} {
		t.Run(name, func(t *testing.T) {
			tx, now, selection := transaction(t)
			user := entity.UserID("bound")

			tx.UserID = &user
			if err := tx.ClaimOperation("start", entity.OperationStart, selection, "", now); err != nil {
				t.Fatal(err)
			}

			if err := tx.AttachChallenge("start", nil, now); err != nil {
				t.Fatal(err)
			}

			if err := tx.ClaimOperation("claim", entity.OperationVerify, selection, "", now); err != nil {
				t.Fatal(err)
			}

			before := tx.Copy()
			proof := entity.Evidence{Requirement: "primary", Method: "oauth", Provider: "google", VerifiedAt: now}
			token := "claim"
			current := now

			switch name {
			case "wrong claim":
				token = "stale"
			case "wrong subject":
				user = "other"
			case "wrong selection":
				proof.Provider = "github"
			case "future proof":
				proof.VerifiedAt = now.Add(time.Second)
			case "stale claim":
				current = now.Add(10 * time.Second)
			case "expired transaction":
				current = tx.ExpiresAt
			}

			// Avoid altering the stored binding through the test variable.
			bound := entity.UserID("bound")

			tx.UserID = &bound
			if err := tx.AcceptEvidence(token, user, proof, current); err == nil {
				t.Fatal("invalid proof accepted")
			}

			if !reflect.DeepEqual(before, tx.Copy()) {
				t.Fatal("rejected transition mutated state")
			}
		})
	}
}

func TestClaimRecoveryAndLimits(t *testing.T) {
	tx, now, selection := transaction(t)
	if err := tx.ClaimOperation("start", entity.OperationStart, selection, "", now); err != nil {
		t.Fatal(err)
	}

	if err := tx.AttachChallenge("start", nil, now); err != nil {
		t.Fatal(err)
	}

	if err := tx.ClaimOperation("claim", entity.OperationVerify, selection, "", now); err != nil {
		t.Fatal(err)
	}

	if err := tx.ClaimOperation("other", entity.OperationVerify, selection, "", now); !errors.Is(err, entity.ErrTransactionBusy) {
		t.Fatal(err)
	}

	if tx.AbandonExpiredClaim(now) {
		t.Fatal("live claim abandoned")
	}

	if !tx.AbandonExpiredClaim(now.Add(10*time.Second)) || tx.Status != entity.TransactionAbandoned {
		t.Fatal("stale claim not abandoned")
	}

	if err := tx.AcceptEvidence("claim", "user", entity.Evidence{}, now.Add(10*time.Second)); !errors.Is(err, entity.ErrTransactionAbandoned) {
		t.Fatal(err)
	}

	tx, now, selection = transaction(t)
	if err := tx.ClaimOperation("start", entity.OperationStart, selection, "", now); err != nil {
		t.Fatal(err)
	}

	if err := tx.AttachChallenge("start", nil, now); err != nil {
		t.Fatal(err)
	}

	for n := 1; n < tx.Settings.MaxAttempts; n++ {
		if err := tx.ClaimOperation("claim", entity.OperationVerify, selection, "", now); err != nil {
			t.Fatal(err)
		}

		if err := tx.RejectOperation("claim", false, now); err != nil {
			t.Fatal(err)
		}
	}

	if err := tx.ClaimOperation("claim", entity.OperationVerify, selection, "", now); !errors.Is(err, entity.ErrAttemptLimit) {
		t.Fatal(err)
	}
}

func TestPolicySnapshotAndBounds(t *testing.T) {
	tx, _, _ := transaction(t)
	copy := tx.Copy()
	root := copy.Policy.Root.(entity.AnyOfRequirement)
	leaf := root.Children[0].(entity.MethodRequirement)
	leaf.Providers[0] = "github"

	original := tx.Policy.Root.(entity.AnyOfRequirement).Children[0].(entity.MethodRequirement)
	if original.Providers[0] != "google" {
		t.Fatal("policy snapshot aliases copy")
	}

	if _, err := tx.Policy.Snapshot(1); err == nil {
		t.Fatal("oversized policy accepted")
	}

	cycle := &entity.AnyOfRequirement{ID: "cycle"}

	cycle.Children = []entity.Requirement{cycle}
	if _, err := (entity.Policy{Root: cycle}).Snapshot(1000); err == nil {
		t.Fatal("cyclic policy accepted")
	}
}

func TestTransactionCheckAttempt(t *testing.T) {
	for _, test := range []struct {
		name    string
		pending *entity.MethodAttempt
		id      string
		wantErr error
	}{
		{name: "no pending attempt", id: "attempt", wantErr: entity.ErrInvalidTransition},
		{name: "different attempt", pending: &entity.MethodAttempt{ID: "attempt"}, id: "other", wantErr: entity.ErrInvalidTransition},
		{name: "empty id", pending: &entity.MethodAttempt{ID: "attempt"}, wantErr: entity.ErrInvalidTransition},
		{name: "matching attempt", pending: &entity.MethodAttempt{ID: "attempt"}, id: "attempt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := entity.Transaction{Pending: test.pending}
			if err := tx.CheckAttempt(test.id); !errors.Is(err, test.wantErr) {
				t.Fatalf("CheckAttempt(%q): got %v, want %v", test.id, err, test.wantErr)
			}
		})
	}
}
