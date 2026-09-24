package entity

import "time"

// Principal describes an authenticated user after policy satisfaction.
// Sessions consume this result without depending on the transaction itself.
type Principal struct {
	TenantID TenantID
	UserID   UserID
	Evidence []Evidence

	AuthenticatedAt time.Time
}
