package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/service"
)

func TestEvaluatorEvaluate(t *testing.T) {
	primary := entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
		entity.MethodRequirement{Method: "password"},
		entity.MethodRequirement{Method: "email_code"},
		entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google", "github"}},
	}}
	mfa := entity.AnyOfRequirement{ID: "mfa", Children: []entity.Requirement{
		entity.MethodRequirement{Method: "totp"},
		entity.MethodRequirement{Method: "telegram_code"},
	}}
	full := entity.AllOfRequirement{Children: []entity.Requirement{primary, mfa}}
	googleOnly := entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
		entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google"}},
	}}
	sameMethod := entity.AllOfRequirement{Children: []entity.Requirement{
		entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
			entity.MethodRequirement{Method: "email_code"},
		}},
		entity.AnyOfRequirement{ID: "mfa", Children: []entity.Requirement{
			entity.MethodRequirement{Method: "email_code"},
		}},
	}}

	tests := []struct {
		name      string
		root      entity.Requirement
		evidence  []entity.Evidence
		remaining entity.Requirement
	}{
		{name: "no evidence leaves complete policy", root: full, remaining: full},
		{
			name: "password satisfies primary", root: primary,
			evidence: []entity.Evidence{{Requirement: "primary", Method: "password"}},
		},
		{
			name: "google satisfies allowed oauth provider", root: googleOnly,
			evidence: []entity.Evidence{{Requirement: "primary", Method: "oauth", Provider: "google"}},
		},
		{
			name: "github cannot satisfy google only", root: googleOnly, remaining: googleOnly,
			evidence: []entity.Evidence{{Requirement: "primary", Method: "oauth", Provider: "github"}},
		},
		{
			name: "oauth proof needs allowed provider", root: googleOnly, remaining: googleOnly,
			evidence: []entity.Evidence{{Requirement: "primary", Method: "oauth"}},
		},
		{
			name: "providerless option rejects provider proof", root: primary, remaining: primary,
			evidence: []entity.Evidence{{Requirement: "primary", Method: "password", Provider: "unexpected"}},
		},
		{
			name: "primary complete leaves mfa", root: full, remaining: mfa,
			evidence: []entity.Evidence{{Requirement: "primary", Method: "email_code"}},
		},
		{
			name: "mfa first leaves primary", root: full, remaining: primary,
			evidence: []entity.Evidence{{Requirement: "mfa", Method: "totp"}},
		},
		{
			name: "primary and totp complete policy", root: full,
			evidence: []entity.Evidence{
				{Requirement: "primary", Method: "password"},
				{Requirement: "mfa", Method: "totp"},
			},
		},
		{
			name: "second provider and alternate mfa complete policy", root: full,
			evidence: []entity.Evidence{
				{Requirement: "primary", Method: "oauth", Provider: "github"},
				{Requirement: "mfa", Method: "telegram_code"},
			},
		},
		{
			name: "evidence is scoped to its requirement", root: sameMethod,
			remaining: sameMethod.Children[1],
			evidence:  []entity.Evidence{{Requirement: "primary", Method: "email_code"}},
		},
		{
			name: "duplicate evidence cannot complete another stage", root: sameMethod,
			remaining: sameMethod.Children[1],
			evidence: []entity.Evidence{
				{Requirement: "primary", Method: "email_code"},
				{Requirement: "primary", Method: "email_code"},
			},
		},
		{
			name: "separate proofs for same method satisfy both stages", root: sameMethod,
			evidence: []entity.Evidence{
				{Requirement: "primary", Method: "email_code"},
				{Requirement: "mfa", Method: "email_code"},
			},
		},
		{
			name: "irrelevant evidence has no effect", root: primary, remaining: primary,
			evidence: []entity.Evidence{
				{Requirement: "other", Method: "password"},
				{Requirement: "primary", Method: "other"},
				{},
			},
		},
		{
			name: "named all preserves scope after partial satisfaction",
			root: entity.AllOfRequirement{ID: "confirmation", Children: []entity.Requirement{
				entity.MethodRequirement{Method: "password"},
				entity.MethodRequirement{Method: "totp"},
			}},
			evidence: []entity.Evidence{{Requirement: "confirmation", Method: "password"}},
			remaining: entity.AllOfRequirement{ID: "confirmation", Children: []entity.Requirement{
				entity.MethodRequirement{Method: "totp"},
			}},
		},
		{
			name: "anonymous groups inherit scope",
			root: entity.AnyOfRequirement{ID: "login", Children: []entity.Requirement{
				entity.AllOfRequirement{Children: []entity.Requirement{
					entity.MethodRequirement{Method: "password"},
					entity.MethodRequirement{Method: "totp"},
				}},
				entity.MethodRequirement{Method: "email_code"},
			}},
			evidence: []entity.Evidence{
				{Requirement: "login", Method: "password"},
				{Requirement: "login", Method: "totp"},
			},
		},
		{
			name:     "nearest named group owns proof",
			root:     entity.AllOfRequirement{ID: "outer", Children: []entity.Requirement{googleOnly}},
			evidence: []entity.Evidence{{Requirement: "primary", Method: "oauth", Provider: "google"}},
		},
		{
			name:      "ancestor proof does not satisfy child scope",
			root:      entity.AllOfRequirement{ID: "outer", Children: []entity.Requirement{googleOnly}},
			remaining: entity.AllOfRequirement{ID: "outer", Children: []entity.Requirement{googleOnly}},
			evidence:  []entity.Evidence{{Requirement: "outer", Method: "oauth", Provider: "google"}},
		},
		{
			name: "any accepts one complete compound branch",
			root: entity.AnyOfRequirement{Children: []entity.Requirement{full,
				entity.AnyOfRequirement{ID: "alternative", Children: []entity.Requirement{
					entity.MethodRequirement{Method: "telegram_mini_app"},
				}},
			}},
			evidence: []entity.Evidence{{Requirement: "alternative", Method: "telegram_mini_app"}},
		},
		{
			name: "pointer nodes are supported",
			root: &entity.AllOfRequirement{Children: []entity.Requirement{
				&entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
					&entity.MethodRequirement{Method: "password"},
				}},
			}},
			evidence: []entity.Evidence{{Requirement: "primary", Method: "password"}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tx := entity.Transaction{TenantID: "acme", Intent: "sign_in", Evidence: tt.evidence}
			policy := entity.Policy{TenantID: tx.TenantID, Intent: tx.Intent, Root: tt.root}

			decision, err := (service.Evaluator{}).Evaluate(context.Background(), tx, policy)
			if err != nil {
				t.Fatal(err)
			}

			want := entity.Decision{Satisfied: tt.remaining == nil, Remaining: tt.remaining}
			if !reflect.DeepEqual(decision, want) {
				t.Fatalf("decision = %#v, want %#v", decision, want)
			}

			if decision.Remaining != nil {
				policy.Root = decision.Remaining

				again, err := (service.Evaluator{}).Evaluate(context.Background(), tx, policy)
				if err != nil || !reflect.DeepEqual(again, decision) {
					t.Fatalf("remaining policy changed on reevaluation: %#v, %v", again, err)
				}
			}
		})
	}
}

// Embedding can expose a domain marker through an otherwise unsupported node.
type unsupportedRequirement struct{ entity.Requirement }

func TestEvaluatorRejectsMalformedPolicies(t *testing.T) {
	cycle := &entity.AllOfRequirement{}
	cycle.Children = []entity.Requirement{cycle}
	valueCycle := entity.AllOfRequirement{Children: make([]entity.Requirement, 1)}
	valueCycle.Children[0] = valueCycle
	valid := entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
		entity.MethodRequirement{Method: "password"},
	}}

	tests := []struct {
		name string
		root entity.Requirement
	}{
		{name: "nil root"},
		{name: "typed nil method", root: (*entity.MethodRequirement)(nil)},
		{name: "typed nil any", root: (*entity.AnyOfRequirement)(nil)},
		{name: "typed nil all", root: (*entity.AllOfRequirement)(nil)},
		{name: "unsupported node", root: unsupportedRequirement{}},
		{name: "empty all", root: entity.AllOfRequirement{ID: "primary"}},
		{name: "empty any", root: entity.AnyOfRequirement{ID: "primary"}},
		{name: "unscoped method", root: entity.MethodRequirement{Method: "password"}},
		{name: "unscoped nested method", root: entity.AnyOfRequirement{Children: []entity.Requirement{
			entity.MethodRequirement{Method: "password"},
		}}},
		{name: "empty method", root: entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
			entity.MethodRequirement{},
		}}},
		{name: "empty provider after allowed provider", root: entity.AnyOfRequirement{
			ID: "primary", Children: []entity.Requirement{
				entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google", ""}},
			},
		}},
		{name: "duplicate group IDs", root: entity.AllOfRequirement{Children: []entity.Requirement{valid, valid}}},
		{name: "invalid branch after satisfied any alternative", root: entity.AnyOfRequirement{
			Children: []entity.Requirement{valid, nil},
		}},
		{name: "duplicate ID after satisfied any alternative", root: entity.AnyOfRequirement{
			Children: []entity.Requirement{valid, valid},
		}},
		{name: "pointer cycle", root: cycle},
		{name: "value cycle", root: valueCycle},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tx := entity.Transaction{TenantID: "acme", Intent: "sign_in", Evidence: []entity.Evidence{
				{Requirement: "primary", Method: "password"},
				{Requirement: "primary", Method: "oauth", Provider: "google"},
			}}
			policy := entity.Policy{TenantID: tx.TenantID, Intent: tx.Intent, Root: tt.root}

			decision, err := (service.Evaluator{}).Evaluate(context.Background(), tx, policy)
			if !errors.Is(err, service.ErrInvalidPolicy) {
				t.Fatalf("error = %v, want ErrInvalidPolicy", err)
			}

			if decision.Satisfied || decision.Remaining != nil {
				t.Fatalf("invalid policy returned a usable decision: %#v", decision)
			}
		})
	}
}

func TestEvaluatorChecksPolicyBindingAndCancellation(t *testing.T) {
	root := entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{
		entity.MethodRequirement{Method: "password"},
	}}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name   string
		ctx    context.Context
		tenant entity.TenantID
		intent entity.Intent
		want   error
	}{
		{name: "different tenant", ctx: context.Background(), tenant: "partner", intent: "sign_in", want: service.ErrPolicyMismatch},
		{name: "different intent", ctx: context.Background(), tenant: "acme", intent: "update", want: service.ErrPolicyMismatch},
		{name: "missing policy tenant", ctx: context.Background(), intent: "sign_in", want: service.ErrInvalidPolicy},
		{name: "missing policy intent", ctx: context.Background(), tenant: "acme", want: service.ErrInvalidPolicy},
		{name: "canceled", ctx: canceled, tenant: "acme", intent: "sign_in", want: context.Canceled},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tx := entity.Transaction{TenantID: "acme", Intent: "sign_in"}
			policy := entity.Policy{TenantID: tt.tenant, Intent: tt.intent, Root: root}

			decision, err := (service.Evaluator{}).Evaluate(tt.ctx, tx, policy)
			if !errors.Is(err, tt.want) || decision.Satisfied || decision.Remaining != nil {
				t.Fatalf("got %#v, %v; want zero decision and %v", decision, err, tt.want)
			}
		})
	}
}

func TestEvaluatorRemainingDoesNotAliasPolicy(t *testing.T) {
	leaf := &entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google"}}
	root := &entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{leaf}}
	policy := entity.Policy{TenantID: "acme", Intent: "sign_in", Root: root}
	tx := entity.Transaction{TenantID: "acme", Intent: "sign_in"}

	decision, err := (service.Evaluator{}).Evaluate(context.Background(), tx, policy)
	if err != nil {
		t.Fatal(err)
	}

	remaining, ok := decision.Remaining.(entity.AnyOfRequirement)
	if !ok {
		t.Fatalf("unexpected remaining type: %T", decision.Remaining)
	}

	remainingLeaf, ok := remaining.Children[0].(entity.MethodRequirement)
	if !ok {
		t.Fatalf("unexpected child type: %T", remaining.Children[0])
	}

	remainingLeaf.Providers[0] = "github"

	remaining.Children[0] = entity.MethodRequirement{Method: "password"}
	if root.Children[0] != leaf || leaf.Method != "oauth" || leaf.Providers[0] != "google" {
		t.Fatal("changing the decision mutated the input policy")
	}
}

func TestEvaluatorMaximumDepth(t *testing.T) {
	var root entity.Requirement = entity.MethodRequirement{Method: "password"}
	for depth := 1; depth < service.MaxRequirementDepth; depth++ {
		root = entity.AnyOfRequirement{Children: []entity.Requirement{root}}
	}

	outer, ok := root.(entity.AnyOfRequirement)
	if !ok {
		t.Fatal("expected group")
	}

	outer.ID = "primary"
	tx := entity.Transaction{TenantID: "acme", Intent: "sign_in"}

	policy := entity.Policy{TenantID: tx.TenantID, Intent: tx.Intent, Root: outer}
	if _, err := (service.Evaluator{}).Evaluate(context.Background(), tx, policy); err != nil {
		t.Fatalf("policy at maximum depth rejected: %v", err)
	}

	policy.Root = entity.AnyOfRequirement{Children: []entity.Requirement{outer}}
	if _, err := (service.Evaluator{}).Evaluate(context.Background(), tx, policy); !errors.Is(err, service.ErrInvalidPolicy) {
		t.Fatalf("policy beyond maximum depth accepted: %v", err)
	}
}
