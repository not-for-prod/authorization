package application_test

import (
	"testing"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
)

func TestCreateTransactionIntentValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		intent entity.Intent
		valid  bool
	}{
		{entity.IntentSignUp, true},
		{entity.IntentSignIn, true},
		{entity.IntentUpdate, true},
		{entity.IntentDelete, true},
		{"", false},
		{"authenticate", false},
		{"create", false},
		{"custom", false},
	} {
		t.Run(string(tc.intent), func(t *testing.T) {
			request := application.CreateTransactionRequest{TenantID: "demo", Intent: tc.intent}
			if err := request.Verify(); (err == nil) != tc.valid {
				t.Fatalf("Verify() = %v, want valid = %v", err, tc.valid)
			}
		})
	}
}
