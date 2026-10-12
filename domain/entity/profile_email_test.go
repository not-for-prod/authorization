package entity_test

import (
	"testing"

	"github.com/not-for-prod/authorization/domain/entity"
)

func TestAttachEmailProtectsUserAndEstablishedAddress(t *testing.T) {
	user := entity.User{ID: "a", Status: entity.UserStatusActive}
	proof := entity.VerifiedIdentity{Kind: "email", Issuer: "atlas/email", Subject: "person@example.com"}
	identity, err := user.AttachEmail(proof, "", "email")
	if err != nil || identity.UserID != "a" || identity.Subject != proof.Subject {
		t.Fatal(identity, err)
	}
	if _, err := user.AttachEmail(proof, "other@example.com", "email"); err == nil {
		t.Fatal("replacement allowed")
	}
	user.Status = entity.UserStatusDeleted
	if _, err := user.AttachEmail(proof, "", "email"); err == nil {
		t.Fatal("deleted user linked")
	}
}
func TestProviderEmailNormalizationDiscardsMalformedClaims(t *testing.T) {
	for _, value := range []string{"", "not-email", "Alice <alice@example.com>"} {
		if entity.NormalizeVerifiedEmail(value) != "" {
			t.Fatal(value)
		}
	}
	if got := entity.NormalizeVerifiedEmail(" Alice@Example.com "); got != "alice@example.com" {
		t.Fatal(got)
	}
}
