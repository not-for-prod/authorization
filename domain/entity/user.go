package entity

import (
	"time"

	"github.com/not-for-prod/proterror/proterror"
)

// UserID identifies the canonical internal subject, independently of the
// external or local identity used to authenticate.
type UserID string

// UserStatus describes a user's lifecycle state.
// Sign-up assigns active; account deletion assigns deleted.
type UserStatus string

// UserStatusActive is the initial status assigned by the sign-up flow.
const UserStatusActive UserStatus = "active"

// UserStatusDeleted prevents further application access while retaining identity ownership.
const UserStatusDeleted UserStatus = "deleted"

// User is the canonical internal subject to which authentication identities resolve.
type User struct {
	ID        UserID
	Status    UserStatus
	CreatedAt time.Time
}

// Delete permanently disables this authentication subject without removing its identities.
func (u *User) Delete() error {
	if u.Status != UserStatusActive {
		return &proterror.FailedPrecondition{}
	}
	u.Status = UserStatusDeleted
	return nil
}

// IdentityID identifies a link between an external or local identifier and a user.
type IdentityID string

// IdentityKind describes the kind of external or local identity.
// Concrete kinds will be defined with identity management behavior.
type IdentityKind string

// Identity links an issuer's stable subject identifier to a canonical user.
// The issuer/subject pair must be unique across identities when persisted.
// Matching email addresses reported by providers do not imply a shared user.
type Identity struct {
	ID     IdentityID
	UserID UserID

	Kind    IdentityKind
	Issuer  string
	Subject string

	// PasswordHash is an optional encoded password hash provisioned by the host.
	// Empty means password authentication is not enrolled. Never log or expose it
	// in public responses, evidence, or events. Hash algorithms belong to drivers.
	PasswordHash string

	CreatedAt time.Time
}
