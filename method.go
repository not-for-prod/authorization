package authorization

import "github.com/not-for-prod/authorization/domain/gateway"

// AuthenticationMethod is a host-configured authentication driver.
type AuthenticationMethod = gateway.Method

// CallbackMethod exposes the gateway contract for host implementations.
type CallbackMethod = gateway.CallbackMethod

// MethodInput exposes the gateway contract for host implementations.
type MethodInput = gateway.MethodInput

// StartResult exposes the gateway contract for host implementations.
type StartResult = gateway.StartResult

// VerificationResult exposes the gateway contract for host implementations.
type VerificationResult = gateway.VerificationResult

// VerifiedIdentity exposes the gateway contract for host implementations.
type VerifiedIdentity = gateway.VerifiedIdentity
