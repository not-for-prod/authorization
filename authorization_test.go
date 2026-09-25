package authorization_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/not-for-prod/proterror/proterror"

	auth "github.com/not-for-prod/authorization"
	"github.com/not-for-prod/authorization/internal/testutil/authfixture"
)

func dependencies(f *authfixture.Fixture) auth.Dependencies {
	return auth.Dependencies{Events: f.Store, Transactions: f.Store, TransactionManager: f.Store, Users: f.Users,
		Identities: f.Identities, Access: f.Config.Access, Eligible: f.Config.Eligible}
}

func TestConstructorValidation(t *testing.T) {
	f := authfixture.New()
	driver := f.Methods["password"]
	policy := auth.WithPolicy(auth.Any("primary", auth.Method(driver)))
	validTenant := auth.WithTenant("demo", auth.WithIntent("sign_in", policy))
	var nilDriver *structDriver
	cases := []struct {
		name string
		opts []auth.Option
	}{
		{"no tenants", nil},
		{"nil option", []auth.Option{nil}},
		{"empty tenant", []auth.Option{auth.WithTenant("", auth.WithIntent("sign_in", policy))}},
		{"duplicate tenant", []auth.Option{validTenant, validTenant}},
		{"no intents", []auth.Option{auth.WithTenant("demo")}},
		{"nil tenant option", []auth.Option{auth.WithTenant("demo", nil)}},
		{"unsupported intent", []auth.Option{auth.WithTenant("demo", auth.WithIntent("custom", policy))}},
		{"duplicate intent", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", policy), auth.WithIntent("sign_in", policy))}},
		{"nil intent option", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", nil))}},
		{"missing policy", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in"))}},
		{"duplicate policy", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", policy, policy))}},
		{"zero policy", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(auth.PolicyNode{})))}},
		{"empty group", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(auth.Any("primary"))))}},
		{"unscoped method", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(auth.Method(driver))))}},
		{"nil driver", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(auth.Any("primary", auth.Method(nil)))))}},
		{"typed nil driver", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(auth.Any("primary", auth.Method(nilDriver)))))}},
		{"empty providers", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(auth.Any("primary", auth.Method(driver, auth.WithProviders())))))}},
		{"empty provider ID", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(auth.Any("primary", auth.Method(driver, auth.WithProviders(""))))))}},
		{"nil method option", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(auth.Any("primary", auth.Method(driver, nil)))))}},
		{"missing session issuer", []auth.Option{auth.WithTenant("demo", auth.WithIntent("sign_in", policy, auth.WithSessionIssuance()))}},
		{"nil session issuer", []auth.Option{auth.WithTenant("demo", auth.WithSessionManager(nil), auth.WithIntent("sign_in", policy))}},
		{"nil clock", []auth.Option{validTenant, auth.WithClock(nil)}},
		{"invalid limits", []auth.Option{validTenant, auth.WithLimits(auth.TransactionSettings{})}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]auth.Option{auth.WithLimits(f.Config.Limits)}, tc.opts...)
			result, err := auth.New(dependencies(f), opts...)
			var invalid *proterror.InvalidArgument
			if result != nil || !errors.As(err, &invalid) {
				t.Fatalf("expected nil service and InvalidArgument, got %v, %v", result, err)
			}
		})
	}
}

// Embedding an interface deliberately makes every method except Name unusable:
// constructor validation must never invoke provider I/O.
type structDriver struct {
	auth.AuthenticationMethod
	label string
}

func (d *structDriver) Name() auth.MethodID { return auth.MethodID(d.label) }

func TestDriverBindingScope(t *testing.T) {
	f := authfixture.New()
	first, second := &structDriver{label: "password"}, &structDriver{label: "password"}
	intent := func(name auth.Intent, driver auth.AuthenticationMethod) auth.TenantOption {
		return auth.WithIntent(name, auth.WithPolicy(auth.Any("primary", auth.Method(driver))))
	}
	cases := []struct {
		name      string
		opts      []auth.Option
		wantError bool
	}{
		{"shared instance across intents", []auth.Option{auth.WithTenant("demo", intent("sign_in", first), intent("update", first))}, false},
		{"conflicting instances", []auth.Option{auth.WithTenant("demo", intent("sign_in", first), intent("update", second))}, true},
		{"separate tenant instances", []auth.Option{auth.WithTenant("first", intent("sign_in", first)), auth.WithTenant("second", intent("sign_in", second))}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := auth.New(dependencies(f), append([]auth.Option{auth.WithLimits(f.Config.Limits)}, tc.opts...)...)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPublicSessionFlow(t *testing.T) {
	for _, issueSession := range []bool{false, true} {
		name := "principal only"
		if issueSession {
			name = "explicit session issuer"
		}
		t.Run(name, func(t *testing.T) {
			f := authfixture.New()
			policy := auth.All(
				auth.Any("primary", auth.Method(f.Methods["password"])),
				auth.Any("mfa", auth.Method(f.Methods["totp"])),
			)
			intentOptions := []auth.IntentOption{auth.WithPolicy(policy)}
			if issueSession {
				intentOptions = append(intentOptions, auth.WithSessionIssuance())
			}
			tenantOptions := []auth.TenantOption{auth.WithIntent("sign_in", intentOptions...)}
			if issueSession {
				tenantOptions = append(tenantOptions, auth.WithSessionManager(f.Sessions))
			}
			svc, err := auth.New(dependencies(f), auth.WithLimits(f.Config.Limits), auth.WithClock(f.Now), auth.WithTenant("demo", tenantOptions...))
			if err != nil {
				t.Fatal(err)
			}

			ctx := authfixture.WithCaller(t.Context(), "owner", nil)
			created, err := svc.CreateTransaction(ctx, auth.CreateTransactionRequest{TenantID: "demo", Intent: "sign_in"})
			if err != nil {
				t.Fatal(err)
			}

			for _, selection := range []auth.MethodSelection{{Requirement: "primary", Method: "password"}, {Requirement: "mfa", Method: "totp"}} {
				started, err := svc.StartMethod(ctx, auth.StartMethodRequest{TenantID: "demo", TransactionID: created.Transaction.ID, Selection: selection, Input: authfixture.Input{}})
				if err != nil {
					t.Fatal(err)
				}
				result, err := svc.VerifyMethod(ctx, auth.VerifyMethodRequest{AttemptID: started.AttemptID, TenantID: "demo", TransactionID: created.Transaction.ID,
					Input: authfixture.Input{UserID: "alice"}})
				if err != nil {
					t.Fatal(err)
				}

				if selection.Method == "totp" && (result.Result == nil || (result.Result.Session != nil) != issueSession) {
					t.Fatalf("incorrect completion: %+v", result.Result)
				}
			}
		})
	}
}

func TestPolicyBuilderSnapshotsAndBounds(t *testing.T) {
	f := authfixture.New()
	providers := []auth.ProviderID{"google"}
	children := []auth.PolicyNode{auth.Method(f.Methods["oauth"], auth.WithProviders(providers...))}
	policy := auth.Any("primary", children...)
	providers[0], children[0] = "changed", auth.PolicyNode{}
	option := auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(policy)))
	svc, err := auth.New(dependencies(f), auth.WithLimits(f.Config.Limits), option)
	if err != nil {
		t.Fatal(err)
	}

	ctx := authfixture.WithCaller(context.Background(), "owner", nil)
	created, err := svc.CreateTransaction(ctx, auth.CreateTransactionRequest{TenantID: "demo", Intent: "sign_in"})
	if err != nil {
		t.Fatal(err)
	}

	if got := created.Transaction.AvailableMethods; len(got) != 1 || got[0].Provider != "google" {
		t.Fatalf("policy retained mutable caller data: %+v", got)
	}

	limits := f.Config.Limits
	limits.MaxPolicyNodes = 2
	if _, err := auth.New(dependencies(f), auth.WithLimits(limits), option); err == nil {
		t.Fatal("provider entries must count toward policy size limit")
	}

	nested := auth.Any("primary", auth.Method(f.Methods["password"]))
	for range 65 {
		nested = auth.All(nested)
	}
	if _, err := auth.New(dependencies(f), auth.WithLimits(f.Config.Limits), auth.WithTenant("demo", auth.WithIntent("sign_in", auth.WithPolicy(nested)))); err == nil {
		t.Fatal("expected depth limit")
	}

	// An option can be applied to another constructor without retaining prior bindings.
	if _, err := auth.New(dependencies(f), auth.WithLimits(f.Config.Limits), option); err != nil {
		t.Fatal(err)
	}
}

func TestTypedNilDependency(t *testing.T) {
	f := authfixture.New()
	deps := dependencies(f)
	var store *authfixture.Store
	deps.Transactions = store
	if service, err := auth.New(deps); err == nil || service != nil {
		t.Fatal("typed nil dependency was accepted")
	}
}

func TestRequiredUser(t *testing.T) {
	f := authfixture.New()
	svc, err := auth.New(dependencies(f), auth.WithLimits(f.Config.Limits), auth.WithClock(func() time.Time { return f.Now() }),
		auth.WithTenant("demo", auth.WithIntent("update", auth.WithAuthenticatedUserRequired(),
			auth.WithPolicy(auth.Any("mfa", auth.Method(f.Methods["totp"]))))))
	if err != nil {
		t.Fatal(err)
	}

	ctx := authfixture.WithCaller(t.Context(), "owner", nil)
	if _, err := svc.CreateTransaction(ctx, auth.CreateTransactionRequest{TenantID: "demo", Intent: "update"}); err == nil {
		t.Fatal("step-up accepted without trusted user")
	}
}
