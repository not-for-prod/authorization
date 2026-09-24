package mock

import (
	"authorization"
)

// Fixed test credentials, not real password hashing or TOTP verification.
type Input struct{ Username, Password, Code string }

func (Input) AuthenticationInput() {}

type Method struct{ id authorization.MethodID }

// NewPassword provides Alice's fixed demo password proof.
func NewPassword() *Method { return &Method{id: "password"} }

// NewTOTP provides Alice's fixed demo MFA proof. It is not real TOTP verification.
func NewTOTP() *Method { return &Method{id: "totp"} }
