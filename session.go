package authorization

import "authorization/domain/entity"

// SessionID exposes the entity contract for host implementations.
type SessionID = entity.SessionID

// Session exposes the entity contract for host implementations.
type Session = entity.Session

// IssuedSession exposes the entity contract for host implementations.
type IssuedSession = entity.IssuedSession

// Principal exposes the entity contract for host implementations.
type Principal = entity.Principal
