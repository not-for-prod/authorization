package authorization_test

import (
	"testing"

	pb "github.com/not-for-prod/authorization/generated/pb/authorization/v1"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/encoding/protojson"
)

// Exercise the HTTP JSON shape and compiled validation rules together, including
// presence semantics that differ between omitted and explicitly empty values.
func TestVerifyMethodRequestValidation(t *testing.T) {
	t.Parallel()

	validator, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name  string
		body  string
		valid bool
	}{
		{name: "missing attempt", body: `{"tenantId":"acme","transactionId":"tx_1","input":{}}`},
		{
			name:  "direct method without challenge or provider",
			body:  `{"tenantId":"acme","transactionId":"tx_1","attemptId":"attempt_1","input":{"password":"example"}}`,
			valid: true,
		},
		{
			name:  "challenge method with custom provider and input",
			body:  `{"tenantId":"acme","transactionId":"tx_1","attemptId":"attempt_1","challengeId":"ch_1","input":{"subject":"9007199254740993"}}`,
			valid: true,
		},
		{
			name: "explicitly empty challenge",
			body: `{"tenantId":"acme","transactionId":"tx_1","attemptId":"attempt_1","challengeId":"","input":{}}`,
		},
		{
			name:  "selection resolved from attempt",
			body:  `{"tenantId":"acme","transactionId":"tx_1","attemptId":"attempt_1","input":{}}`,
			valid: true,
		},
		{
			name: "missing driver input",
			body: `{"tenantId":"acme","transactionId":"tx_1","attemptId":"attempt_1"}`,
		},
		{
			name:  "empty object allowed for a driver without input",
			body:  `{"tenantId":"acme","transactionId":"tx_1","attemptId":"attempt_1","input":{}}`,
			valid: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var request pb.VerifyMethodRequest
			if err := protojson.Unmarshal([]byte(tt.body), &request); err != nil {
				t.Fatal(err)
			}

			if err := validator.Validate(&request); (err == nil) != tt.valid {
				t.Fatalf("Validate() = %v, want valid = %v", err, tt.valid)
			}
		})
	}
}

func TestCreateTransactionIntentValidation(t *testing.T) {
	t.Parallel()
	validator, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		value pb.Intent
		valid bool
	}{
		{"sign_up", pb.Intent_INTENT_SIGN_UP, true},
		{"sign_in", pb.Intent_INTENT_SIGN_IN, true},
		{"update", pb.Intent_INTENT_UPDATE, true},
		{"delete", pb.Intent_INTENT_DELETE, true},
		{"unspecified", pb.Intent_INTENT_UNSPECIFIED, false},
		{"unknown", pb.Intent(99), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := &pb.CreateTransactionRequest{TenantId: "demo", Intent: tc.value}
			if err := validator.Validate(request); (err == nil) != tc.valid {
				t.Fatalf("Validate() = %v, want valid = %v", err, tc.valid)
			}
		})
	}
}
