package authenticationservice

import (
	"context"
	"errors"

	"github.com/not-for-prod/authorization/domain/application"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	pb "github.com/not-for-prod/authorization/generated/pb/authorization/v1"

	"buf.build/go/protovalidate"
	"github.com/not-for-prod/proterror/proterror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ pb.AuthenticationServiceServer = (*Implementation)(nil)

// Decoder belongs to the host: decode driver-owned input, validate its shape,
// and perform no external side effects. Verify and callback selection come from storage.
// Never return credentials or raw provider payloads in errors.
type Decoder func(context.Context, entity.TenantID, entity.MethodSelection, entity.OperationKind, *structpb.Struct) (gateway.MethodInput, error)

type Implementation struct {
	app       application.Authentication
	decode    Decoder
	validator protovalidate.Validator
}

func NewImplementation(app application.Authentication, decode Decoder) (*Implementation, error) {
	if app == nil || decode == nil {
		return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("application and input decoder are required"))
	}

	validator, err := protovalidate.New()
	if err != nil {
		return nil, errors.Join(&proterror.Internal{}, err)
	}

	return &Implementation{app: app, decode: decode, validator: validator}, nil
}

// Validation is in the handler because Clay HTTP and direct Go calls may bypass
// gRPC middleware. Validation failures intentionally omit raw field values.
func (i *Implementation) validate(req proto.Message) error {
	if req == nil || !req.ProtoReflect().IsValid() {
		return status.Error(codes.InvalidArgument, "invalid request")
	}

	if err := i.validator.Validate(req); err != nil {
		return status.Error(codes.InvalidArgument, "invalid request")
	}

	return nil
}

// transportError maps classification, not internal error text, onto the wire.
func transportError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, application.ErrAccountNotFound) {
		return status.Error(codes.NotFound, "account_not_found")
	}

	code := codes.Internal

	var classified interface {
		error
		Code() codes.Code
	}

	switch {
	case errors.Is(err, context.Canceled):
		code = codes.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		code = codes.DeadlineExceeded
	case errors.As(err, &classified):
		code = classified.Code()
	default:
		if s, ok := status.FromError(err); ok {
			code = s.Code()
		}
	}

	if code == codes.OK {
		code = codes.Internal
	}

	return status.Error(code, code.String())
}

func toSelection(s *pb.MethodSelection) entity.MethodSelection {
	return entity.MethodSelection{Requirement: entity.RequirementID(s.GetRequirementId()), Method: entity.MethodID(s.GetMethodId()), Provider: entity.ProviderID(s.GetProviderId())}
}

func fromSelection(s entity.MethodSelection) *pb.MethodSelection {
	return &pb.MethodSelection{RequirementId: string(s.Requirement), MethodId: string(s.Method), ProviderId: string(s.Provider)}
}

func fromTransaction(tx application.TransactionProgress) *pb.Transaction {
	result := &pb.Transaction{Status: string(tx.Status), Id: string(tx.ID), TenantId: string(tx.TenantID), Intent: intentToProto(tx.Intent),
		CreatedAt: timestamppb.New(tx.CreatedAt), ExpiresAt: timestamppb.New(tx.ExpiresAt), PolicySatisfied: tx.PolicySatisfied}
	if tx.PendingAttempt != nil {
		result.PendingAttempt = &pb.PendingMethod{AttemptId: tx.PendingAttempt.AttemptID, Selection: fromSelection(tx.PendingAttempt.Selection), ChallengeId: string(tx.PendingAttempt.ChallengeID), Ready: tx.PendingAttempt.Ready}
	}

	for _, selection := range tx.AvailableMethods {
		result.AvailableMethods = append(result.AvailableMethods, fromSelection(selection))
	}

	return result
}

func fromResult(result *application.AuthenticationResult) *pb.AuthenticationResult {
	if result == nil {
		return nil
	}

	response := &pb.AuthenticationResult{Principal: &pb.Principal{TenantId: string(result.Principal.TenantID),
		UserId: string(result.Principal.UserID), AuthenticatedAt: timestamppb.New(result.Principal.AuthenticatedAt)}}
	if result.Session != nil {
		session := result.Session
		response.Session = &pb.IssuedSession{Id: string(session.Session.ID), Token: session.Token,
			IssuedAt: timestamppb.New(session.Session.IssuedAt), ExpiresAt: timestamppb.New(session.Session.ExpiresAt)}
	}

	return response
}

func intentToProto(intent entity.Intent) pb.Intent {
	switch intent {
	case entity.IntentSignUp:
		return pb.Intent_INTENT_SIGN_UP
	case entity.IntentSignIn:
		return pb.Intent_INTENT_SIGN_IN
	case entity.IntentUpdate:
		return pb.Intent_INTENT_UPDATE
	case entity.IntentDelete:
		return pb.Intent_INTENT_DELETE
	default:
		return pb.Intent_INTENT_UNSPECIFIED
	}
}
