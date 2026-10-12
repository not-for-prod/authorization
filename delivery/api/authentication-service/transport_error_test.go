package authenticationservice

import (
	"errors"
	"testing"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/proterror/proterror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTransportErrorAccountNotFound(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		message string
	}{
		{"unregistered identity", errors.Join(&proterror.NotFound{}, application.ErrAccountNotFound), "account_not_found"},
		{"missing transaction", &proterror.NotFound{}, "NotFound"},
		{"private detail", errors.Join(&proterror.NotFound{}, errors.New("private storage detail")), "NotFound"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := status.Convert(transportError(tc.err))
			if result.Code() != codes.NotFound || result.Message() != tc.message {
				t.Fatalf("unexpected transport error: %v", result)
			}
		})
	}
}
