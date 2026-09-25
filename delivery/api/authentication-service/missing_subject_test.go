package authenticationservice_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/internal/testutil/authfixture"

	"github.com/not-for-prod/proterror/proterror"
)

func TestMissingSubjectReturnsNotFound(t *testing.T) {
	for _, scenario := range []string{"user", "identity"} {
		t.Run(scenario, func(t *testing.T) {
			f := authfixture.New()
			body := `{"input":{"userId":"alice"}}`
			if scenario == "user" {
				f.Users.GetFunc = func(context.Context, entity.UserID, ...repository.SelectOption) (*entity.User, error) {
					return nil, &proterror.NotFound{}
				}
			} else {
				f.Identities.GetBySubjectFunc = func(context.Context, string, string, ...repository.SelectOption) (*entity.Identity, error) {
					return nil, repository.ErrNotFound
				}
			}
			_, h := handler(t, f)
			base := "/v1/tenants/demo/auth/transactions"
			created := request(t, h, "browser", http.MethodPost, base, `{"intent":"INTENT_SIGN_IN"}`, http.StatusOK)
			id := created["transaction"].(map[string]any)["id"].(string)
			startBody := `{"selection":{"requirementId":"primary","methodId":"password"},"input":{}}`
			if scenario == "identity" {
				startBody = `{"selection":{"requirementId":"primary","methodId":"oauth","providerId":"google"},"input":{}}`
			}
			started := request(t, h, "browser", http.MethodPost, base+"/"+id+":startMethod", startBody, http.StatusOK)
			body = `{"attemptId":"` + started["attemptId"].(string) + `",` + body[1:]
			if scenario == "identity" {
				body = `{"challengeId":"` + started["challenge"].(map[string]any)["id"].(string) + `",` + body[1:]
			}
			response := request(t, h, "browser", http.MethodPost, base+"/"+id+":verifyMethod", body, http.StatusNotFound)
			if response["message"] != "NotFound" {
				t.Fatalf("unexpected error: %+v", response)
			}
			saved, err := f.Store.Get(context.Background(), "demo", entity.TransactionID(id), "browser")
			if err != nil {
				t.Fatal(err)
			}
			if len(saved.Evidence) != 0 || saved.Claim != nil || f.SessionCount() != 0 {
				t.Fatal("failed resolution advanced authentication")
			}
		})
	}
}
