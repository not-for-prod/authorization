package authenticationservice_test

import (
	"context"
	"net/http"
	"testing"

	"authorization/domain/entity"
	pb "authorization/generated/pb/authorization/v1"
	"authorization/internal/testutil/authfixture"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestHTTPGetAuthPolicy(t *testing.T) {
	f := authfixture.New()
	root := f.Policy.Root.(entity.AllOfRequirement)
	primary := root.Children[0].(entity.AnyOfRequirement)
	primary.Children[1] = entity.MethodRequirement{Method: "oauth", Providers: []entity.ProviderID{"google", "github"}}
	api, h := handler(t, f)
	response := request(t, h, "browser", http.MethodGet, "/v1/tenants/demo/auth/policy", "", http.StatusOK)
	intents := response["intents"].([]any)
	if len(intents) != 1 {
		t.Fatalf("intents: %+v", intents)
	}
	intent := intents[0].(map[string]any)
	if intent["intent"] != "INTENT_SIGN_IN" {
		t.Fatal(intent)
	}
	groups := intent["requirements"].(map[string]any)["all"].([]any)
	if len(groups) != 2 {
		t.Fatal(groups)
	}
	first := groups[0].(map[string]any)
	if first["id"] != "primary" {
		t.Fatal(first)
	}
	options := first["any"].([]any)
	if options[0].(map[string]any)["method"] != "password" {
		t.Fatal(options)
	}
	providers := options[1].(map[string]any)["any"].([]any)
	if len(providers) != 2 || providers[0].(map[string]any)["provider"] != "google" || providers[1].(map[string]any)["provider"] != "github" {
		t.Fatal(providers)
	}
	if groups[1].(map[string]any)["id"] != "mfa" {
		t.Fatal(groups)
	}
	if f.Store.Writes() != 0 || f.SessionCount() != 0 {
		t.Fatal("discovery changed state")
	}
	request(t, h, "", http.MethodGet, "/v1/tenants/demo/auth/policy", "", http.StatusForbidden)
	request(t, h, "browser", http.MethodGet, "/v1/tenants/other/auth/policy", "", http.StatusForbidden)
	for _, req := range []*pb.GetAuthPolicyRequest{nil, {}} {
		if _, err := api.GetAuthPolicy(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("invalid request: %v", err)
		}
	}
}
