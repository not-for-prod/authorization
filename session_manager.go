package authorization

import "github.com/not-for-prod/authorization/domain/gateway"

// SessionManager exposes the gateway contract for host implementations.
type SessionManager = gateway.SessionManager

// CreateSessionRequest exposes the gateway contract for host implementations.
type CreateSessionRequest = gateway.CreateSessionRequest
