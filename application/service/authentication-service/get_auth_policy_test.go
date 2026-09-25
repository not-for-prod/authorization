package authenticationservice_test

import (
	"context"
	"errors"
	"testing"

	authenticationservice "github.com/not-for-prod/authorization/application/service/authentication-service"
	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	configured "github.com/not-for-prod/authorization/driver/policy/configured"
	"github.com/not-for-prod/authorization/internal/testutil/authfixture"
)

func TestGetAuthPolicyVisibilityAndSnapshots(t *testing.T) {
	f := authfixture.New()
	tenant := f.Config.Tenants["demo"]
	tenant.Intents["sign_up"] = authenticationservice.IntentConfig{}
	tenant.Intents["update"] = authenticationservice.IntentConfig{RequireAuthenticatedUser: true}
	policies := make([]entity.Policy, 0, 3)
	for _, intent := range []entity.Intent{"sign_in", "sign_up", "update"} {
		policy := *f.Policy
		policy.Intent = intent
		policies = append(policies, policy)
	}
	repository, err := configured.NewImplementation(policies, f.Config.Limits.MaxPolicyNodes)
	if err != nil {
		t.Fatal(err)
	}
	f.Config.Policies = repository
	svc, err := authenticationservice.NewImplementation(f.Config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authfixture.WithCaller(context.Background(), "browser", nil)
	req := application.GetAuthPolicyRequest{TenantID: "demo"}
	response, err := svc.GetAuthPolicy(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Intents) != 2 || response.Intents[0].Intent != "sign_in" || response.Intents[1].Intent != "sign_up" {
		t.Fatalf("intents: %+v", response.Intents)
	}
	root := response.Intents[0].Root.(entity.AllOfRequirement)
	root.Children[0] = entity.MethodRequirement{Method: "corrupted"}
	again, err := svc.GetAuthPolicy(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Intents[0].Root.(entity.AllOfRequirement).Children[0].(entity.AnyOfRequirement); !ok {
		t.Fatal("returned policy aliases stored configuration")
	}
	user := entity.UserID("alice")
	authenticated, err := svc.GetAuthPolicy(authfixture.WithCaller(ctx, "browser", &user), req)
	if err != nil || len(authenticated.Intents) != 3 {
		t.Fatalf("authenticated discovery: %+v %v", authenticated, err)
	}
	if f.Store.Writes() != 0 || f.SessionCount() != 0 {
		t.Fatal("discovery caused writes")
	}
	for _, method := range f.Methods {
		if len(method.AvailableCalls()) != 0 || len(method.StartCalls()) != 0 || len(method.VerifyCalls()) != 0 {
			t.Fatal("discovery invoked method driver")
		}
	}
	if _, err := svc.GetAuthPolicy(context.Background(), req); err == nil {
		t.Fatal("missing caller accepted")
	}
	if _, err := svc.GetAuthPolicy(ctx, application.GetAuthPolicyRequest{}); err == nil {
		t.Fatal("missing tenant accepted")
	}
	if _, err := svc.GetAuthPolicy(ctx, application.GetAuthPolicyRequest{TenantID: "other"}); err == nil {
		t.Fatal("unknown tenant accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := svc.GetAuthPolicy(canceled, req); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
