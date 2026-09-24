package authenticationservice_test

import (
	"context"
	"testing"

	apiservice "authorization/delivery/api/authentication-service"
	"authorization/domain/application"
	"authorization/domain/entity"
	"authorization/domain/gateway"
	pb "authorization/generated/pb/authorization/v1"
	"authorization/internal/testutil/authfixture"

	"google.golang.org/protobuf/types/known/structpb"
)

type verifyApplication struct {
	application.Authentication
	verify func(context.Context, application.VerifyMethodRequest) (*application.VerifyMethodResponse, error)
}

func (a verifyApplication) VerifyMethod(ctx context.Context, req application.VerifyMethodRequest) (*application.VerifyMethodResponse, error) {
	return a.verify(ctx, req)
}

func TestVerifyDecodesStoredSelection(t *testing.T) {
	selection := entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}
	decoded := false
	app := verifyApplication{verify: func(_ context.Context, req application.VerifyMethodRequest) (*application.VerifyMethodResponse, error) {
		if decoded || req.Input != nil || req.DecodeInput == nil {
			t.Fatal("input decoded before resolving attempt")
		}
		if req.AttemptID != "attempt" || req.TransactionID != "transaction" || req.TenantID != "demo" {
			t.Fatalf("incorrect request: %+v", req)
		}
		input, err := req.DecodeInput(selection)
		if err != nil {
			t.Fatal(err)
		}
		if input != (authfixture.Input{UserID: "alice"}) {
			t.Fatalf("incorrect decoded input: %+v", input)
		}
		return &application.VerifyMethodResponse{}, nil
	}}
	api, err := apiservice.NewImplementation(app, func(_ context.Context, tenant entity.TenantID, actual entity.MethodSelection, operation entity.OperationKind, input *structpb.Struct) (gateway.MethodInput, error) {
		decoded = true
		if tenant != "demo" || actual != selection || operation != entity.OperationVerify || input.Fields["userId"].GetStringValue() != "alice" {
			t.Fatal("incorrect decoder arguments")
		}
		return authfixture.Input{UserID: "alice"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.VerifyMethod(t.Context(), &pb.VerifyMethodRequest{TenantId: "demo", TransactionId: "transaction", AttemptId: "attempt", Input: &structpb.Struct{Fields: map[string]*structpb.Value{"userId": structpb.NewStringValue("alice")}}})
	if err != nil {
		t.Fatal(err)
	}
	if !decoded {
		t.Fatal("proof was not decoded")
	}
}
