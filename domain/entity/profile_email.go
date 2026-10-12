package entity

import (
	"net/mail"
	"strings"

	"github.com/not-for-prod/proterror/proterror"
)

// NormalizeVerifiedEmail discards invalid provider claims without breaking login.
func NormalizeVerifiedEmail(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || len(value) > 254 {
		return ""
	}
	return value
}

// AttachEmail constructs an owned identity after the application locks this user.
func (u User) AttachEmail(proof VerifiedIdentity, current string, id IdentityID) (Identity, error) {
	if u.Status != UserStatusActive || proof.Kind != "email" || proof.Issuer != "atlas/email" ||
		NormalizeVerifiedEmail(proof.Subject) != proof.Subject || proof.Subject == "" || id == "" {
		return Identity{}, &proterror.FailedPrecondition{}
	}
	if current != "" && current != proof.Subject {
		return Identity{}, &proterror.AlreadyExists{}
	}
	return Identity{ID: id, UserID: u.ID, Kind: proof.Kind, Issuer: proof.Issuer, Subject: proof.Subject}, nil
}

// RetainVerifiedEmail accepts only trusted driver metadata for this user's identity.
func (u User) RetainVerifiedEmail(identity Identity, value string) (Identity, error) {
	if u.Status != UserStatusActive || identity.UserID != u.ID || identity.Kind != "oauth" {
		return Identity{}, &proterror.FailedPrecondition{}
	}
	identity.VerifiedEmail = NormalizeVerifiedEmail(value)
	return identity, nil
}
