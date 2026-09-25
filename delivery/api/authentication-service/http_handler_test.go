package authenticationservice_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/not-for-prod/proterror/proterror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	appservice "github.com/not-for-prod/authorization/application/service/authentication-service"
	apiservice "github.com/not-for-prod/authorization/delivery/api/authentication-service"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	pb "github.com/not-for-prod/authorization/generated/pb/authorization/v1"
	"github.com/not-for-prod/authorization/internal/testutil/authfixture"
)

func handler(t *testing.T, f *authfixture.Fixture) (*apiservice.Implementation, http.Handler) {
	t.Helper()
	app, err := appservice.NewImplementation(f.Config)
	if err != nil {
		t.Fatal(err)
	}
	api, err := apiservice.NewImplementation(app, func(ctx context.Context, tenant entity.TenantID, selection entity.MethodSelection, operation entity.OperationKind, input *structpb.Struct) (gateway.MethodInput, error) {
		authfixture.AssertOutsideTransaction(ctx)
		if tenant != "demo" {
			return nil, &proterror.PermissionDenied{}
		}
		// This fixture stands in for a host's real driver-specific decoder.
		if operation == entity.OperationCallback && (selection.Method != "oauth" || selection.Provider != "google") {
			t.Fatal("callback decoder got wrong stored selection")
		}
		user := input.GetFields()["userId"].GetStringValue()
		return authfixture.Input{UserID: entity.UserID(user), Identity: selection.Method == "oauth", Reject: input.GetFields()["reject"].GetBoolValue()}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	httpHandler, err := api.HTTPHandler(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return api, httpHandler
}

func request(t *testing.T, h http.Handler, caller, method, path, body string, want int) map[string]any {
	t.Helper()
	ctx := authfixture.WithCaller(context.Background(), caller, nil)
	req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code != want {
		t.Fatalf("%s %s: status=%d body=%s", method, path, response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-driver-state", "ownerKey", "evidence", "claim"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("private state exposed: %s", response.Body.String())
		}
	}
	return result
}

func TestHTTPPasswordMFAFlow(t *testing.T) {
	f := authfixture.New()
	_, h := handler(t, f)
	base := "/v1/tenants/demo/auth/transactions"
	created := request(t, h, "browser", http.MethodPost, base, `{"intent":"INTENT_SIGN_IN"}`, http.StatusOK)
	tx := created["transaction"].(map[string]any)
	id := tx["id"].(string)
	path := base + "/" + id
	attempt := request(t, h, "browser", http.MethodPost, path+":startMethod", `{"selection":{"requirementId":"primary","methodId":"password"},"input":{}}`, http.StatusOK)
	primary := request(t, h, "browser", http.MethodPost, path+":verifyMethod", `{"attemptId":"`+attempt["attemptId"].(string)+`","input":{"userId":"alice"}}`, http.StatusOK)
	if primary["result"] != nil {
		t.Fatal("session issued before MFA")
	}
	attempt = request(t, h, "browser", http.MethodPost, path+":startMethod", `{"selection":{"requirementId":"mfa","methodId":"totp"},"input":{}}`, http.StatusOK)
	finished := request(t, h, "browser", http.MethodPost, path+":verifyMethod", `{"attemptId":"`+attempt["attemptId"].(string)+`","input":{"userId":"alice"}}`, http.StatusOK)
	result := finished["result"].(map[string]any)
	principal := result["principal"].(map[string]any)
	if principal["userId"] != "alice" || principal["tenantId"] != "demo" || result["session"].(map[string]any)["token"] == "" {
		t.Fatal("wrong completion")
	}
	writes := f.Store.Writes()
	progress := request(t, h, "browser", http.MethodGet, path, "", http.StatusOK)
	bytes, err := json.Marshal(progress)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), "token") || strings.Contains(string(bytes), "alice") || f.Store.Writes() != writes {
		t.Fatal("Get leaked credentials or wrote state")
	}
	if f.SessionCount() != 1 {
		t.Fatal("incorrect session count")
	}
}

func TestHTTPCallbackFlow(t *testing.T) {
	f := authfixture.New()
	_, h := handler(t, f)
	base := "/v1/tenants/demo/auth/transactions"
	created := request(t, h, "browser", http.MethodPost, base, `{"intent":"INTENT_SIGN_IN"}`, http.StatusOK)
	path := base + "/" + created["transaction"].(map[string]any)["id"].(string)
	started := request(t, h, "browser", http.MethodPost, path+":startMethod", `{"selection":{"requirementId":"primary","methodId":"oauth","providerId":"google"},"input":{}}`, http.StatusOK)
	challenge := started["challenge"].(map[string]any)
	if started["redirectUrl"] != "https://provider.example/authorize" || challenge["state"] != nil {
		t.Fatal("wrong public challenge")
	}
	callback := path + "/challenges/" + challenge["id"].(string) + ":completeCallback"
	request(t, h, "intruder", http.MethodPost, callback, `{"attemptId":"`+started["attemptId"].(string)+`","input":{"userId":"alice"}}`, http.StatusNotFound)
	if len(f.Methods["oauth"].CallbackCalls()) != 0 {
		t.Fatal("unauthorized callback dispatched")
	}
	request(t, h, "browser", http.MethodPost, callback, `{"attemptId":"`+started["attemptId"].(string)+`","input":{"userId":"alice","method":"password"}}`, http.StatusOK)
	request(t, h, "browser", http.MethodPost, callback, `{"attemptId":"`+started["attemptId"].(string)+`","input":{"userId":"alice"}}`, http.StatusOK)
	attempt := request(t, h, "browser", http.MethodPost, path+":startMethod", `{"selection":{"requirementId":"mfa","methodId":"totp"},"input":{}}`, http.StatusOK)
	request(t, h, "browser", http.MethodPost, path+":verifyMethod", `{"attemptId":"`+attempt["attemptId"].(string)+`","input":{"userId":"alice"}}`, http.StatusOK)
	if len(f.Methods["oauth"].CallbackCalls()) != 1 || f.SessionCount() != 1 {
		t.Fatal("callback not consumed exactly once")
	}
}

func TestHTTPValidationAuthorizationAndErrorPrivacy(t *testing.T) {
	f := authfixture.New()
	api, h := handler(t, f)
	base := "/v1/tenants/demo/auth/transactions"
	request(t, h, "browser", http.MethodPost, base, `{}`, http.StatusBadRequest)
	request(t, h, "browser", http.MethodPost, base, `{"intent":`, http.StatusBadRequest)
	request(t, h, "", http.MethodPost, base, `{"intent":"INTENT_SIGN_IN"}`, http.StatusForbidden)
	created := request(t, h, "browser", http.MethodPost, base, `{"intent":"INTENT_SIGN_IN"}`, http.StatusOK)
	path := base + "/" + created["transaction"].(map[string]any)["id"].(string)
	request(t, h, "browser", http.MethodPost, path+":verifyMethod", `{"input":{}}`, http.StatusBadRequest)
	request(t, h, "browser", http.MethodPost, path+":verifyMethod", `{"selection":{"requirementId":"primary","methodId":"password"}}`, http.StatusBadRequest)
	request(t, h, "other", http.MethodGet, path, "", http.StatusNotFound)
	f.Methods["password"].VerifyFunc = func(context.Context, entity.Transaction, entity.MethodSelection, *entity.Challenge, gateway.MethodInput) (gateway.VerificationResult, error) {
		return gateway.VerificationResult{}, errors.New("sensitive-driver-diagnostic")
	}
	attempt := request(t, h, "browser", http.MethodPost, path+":startMethod", `{"selection":{"requirementId":"primary","methodId":"password"},"input":{}}`, http.StatusOK)
	result := request(t, h, "browser", http.MethodPost, path+":verifyMethod", `{"attemptId":"`+attempt["attemptId"].(string)+`","input":{"userId":"alice"}}`, http.StatusInternalServerError)
	if result["message"] != "Internal" {
		t.Fatalf("internal diagnostic leaked: %+v", result)
	}
	if _, err := api.CreateTransaction(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request: %v", err)
	}
	if _, err := api.StartMethod(context.Background(), &pb.StartMethodRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unvalidated direct handler: %v", err)
	}
}
