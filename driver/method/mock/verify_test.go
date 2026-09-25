package mock_test

import (
	"context"
	"errors"
	"testing"

	"github.com/not-for-prod/authorization"
	"github.com/not-for-prod/authorization/driver/method/mock"
)

func TestDemoProofs(t *testing.T) {
	user := authorization.UserID("alice")
	for _, test := range []struct {
		name   string
		method *mock.Method
		input  mock.Input
		user   *authorization.UserID
		valid  bool
	}{
		{name: "password", method: mock.NewPassword(), input: mock.Input{Username: "alice", Password: "test-password"}, valid: true},
		{name: "wrong password", method: mock.NewPassword(), input: mock.Input{Username: "alice", Password: "wrong"}},
		{name: "totp", method: mock.NewTOTP(), input: mock.Input{Code: "123456"}, user: &user, valid: true},
		{name: "unbound totp", method: mock.NewTOTP(), input: mock.Input{Code: "123456"}},
		{name: "unconfigured method", method: &mock.Method{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection := authorization.MethodSelection{Method: test.method.Name(), Requirement: "proof"}

			proof, err := test.method.Verify(context.Background(), authorization.Transaction{UserID: test.user}, selection, nil, test.input)
			if !test.valid {
				if !errors.Is(err, authorization.ErrProofRejected) {
					t.Fatalf("expected rejected proof: %v", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if proof.UserID == nil || *proof.UserID != "alice" || proof.Evidence.Method != selection.Method || proof.Evidence.Requirement != "proof" {
				t.Fatalf("unexpected proof: %+v", proof)
			}
		})
	}
}
