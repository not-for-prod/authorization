package authorization

import "authorization/domain/application"

// Authentication exposes the application contract for host implementations.
type Authentication = application.Authentication

// Caller exposes the application contract for host implementations.
type Caller = application.Caller

// Access exposes the application contract for host implementations.
type Access = application.Access

// TransactionProgress exposes the application contract for host implementations.
type TransactionProgress = application.TransactionProgress

// AuthenticationResult exposes the application contract for host implementations.
type AuthenticationResult = application.AuthenticationResult

// ChallengeReference exposes the application contract for host implementations.
type ChallengeReference = application.ChallengeReference

// CreateTransactionRequest exposes the application contract for host implementations.
type CreateTransactionRequest = application.CreateTransactionRequest

// CreateTransactionResponse exposes the application contract for host implementations.
type CreateTransactionResponse = application.CreateTransactionResponse

// GetTransactionRequest exposes the application contract for host implementations.
type GetTransactionRequest = application.GetTransactionRequest

// GetTransactionResponse exposes the application contract for host implementations.
type GetTransactionResponse = application.GetTransactionResponse

// StartMethodRequest exposes the application contract for host implementations.
type StartMethodRequest = application.StartMethodRequest

// StartMethodResponse exposes the application contract for host implementations.
type StartMethodResponse = application.StartMethodResponse

// VerifyMethodRequest exposes the application contract for host implementations.
type VerifyMethodRequest = application.VerifyMethodRequest

// VerifyMethodResponse exposes the application contract for host implementations.
type VerifyMethodResponse = application.VerifyMethodResponse

// CompleteCallbackRequest exposes the application contract for host implementations.
type CompleteCallbackRequest = application.CompleteCallbackRequest

// CompleteCallbackResponse exposes the application contract for host implementations.
type CompleteCallbackResponse = application.CompleteCallbackResponse

// GetAuthPolicyRequest selects the tenant for policy discovery.
type GetAuthPolicyRequest = application.GetAuthPolicyRequest

// GetAuthPolicyResponse lists caller-visible configured policies.
type GetAuthPolicyResponse = application.GetAuthPolicyResponse

type PendingMethod = application.PendingMethod
