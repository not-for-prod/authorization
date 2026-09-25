package configuredpolicy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	configuredpolicy "github.com/not-for-prod/authorization/driver/policy/configured"
)

func TestIndependentPolicySnapshots(t *testing.T) {
	providers := []entity.ProviderID{"google"}
	children := []entity.Requirement{entity.MethodRequirement{Method: "oauth", Providers: providers}}
	policy := entity.Policy{TenantID: "demo", Intent: "sign_in", Root: entity.AnyOfRequirement{ID: "primary", Children: children}}

	store, err := configuredpolicy.NewImplementation([]entity.Policy{policy}, 16)
	if err != nil {
		t.Fatal(err)
	}

	providers[0] = "changed"
	children[0] = entity.MethodRequirement{Method: "changed"}

	for range 2 {
		got, err := store.Get(t.Context(), "demo", "sign_in")
		if err != nil {
			t.Fatal(err)
		}

		leaf := got.Root.(entity.AnyOfRequirement).Children[0].(entity.MethodRequirement)
		if leaf.Method != "oauth" || leaf.Providers[0] != "google" {
			t.Fatalf("mutable policy leaked: %+v", leaf)
		}

		leaf.Providers[0] = "changed again"
	}

	for _, key := range [][2]string{{"other", "sign_in"}, {"demo", "other"}} {
		if _, err := store.Get(t.Context(), entity.TenantID(key[0]), entity.Intent(key[1])); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("expected scoped lookup failure, got %v", err)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := store.Get(ctx, "demo", "sign_in"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
