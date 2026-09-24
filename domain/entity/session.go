package entity

import "time"

// SessionID identifies a session independently of its issued credential.
type SessionID string

// Session describes the logical session created for an authenticated principal.
type Session struct {
	ID        SessionID
	Principal Principal

	IssuedAt  time.Time
	ExpiresAt time.Time
}

// IssuedSession pairs a logical session with the credential returned to a caller.
// Token may be a JWT or an opaque token, depending on the session implementation.
type IssuedSession struct {
	Session Session
	Token   string
}
