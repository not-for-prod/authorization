package main

import (
	"context"

	"github.com/not-for-prod/proterror/proterror"
	"google.golang.org/protobuf/types/known/structpb"

	"authorization"
	"authorization/driver/method/logpass"
)

func decodeInput(
	_ context.Context,
	_ authorization.TenantID,
	selection authorization.MethodSelection,
	operation authorization.OperationKind,
	input *structpb.Struct,
) (authorization.MethodInput, error) {
	if input == nil || selection.Method != "password" || selection.Provider != "" {
		return nil, &proterror.InvalidArgument{}
	}

	fields := input.GetFields()
	if operation == authorization.OperationStart {
		if len(fields) != 0 {
			return nil, &proterror.InvalidArgument{}
		}

		return logpass.StartInput{}, nil
	}

	if operation != authorization.OperationVerify {
		return nil, &proterror.InvalidArgument{}
	}

	username := fields["login"].GetStringValue()
	password := fields["password"].GetStringValue()
	if len(fields) != 2 || username == "" || password == "" {
		return nil, &proterror.InvalidArgument{}
	}

	return logpass.VerifyInput{Login: username, Password: password}, nil
}
