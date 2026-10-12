package goth

import "testing"

func TestGoogleEmailRequiresVerification(t *testing.T) {
	for _, claims := range []map[string]interface{}{nil, {"email_verified": false}, {"email_verified": "true"}} {
		if verifiedGoogleEmail("a@example.com", claims) != "" {
			t.Fatal("unverified claim accepted")
		}
	}
	if got := verifiedGoogleEmail("A@Example.com", map[string]interface{}{"email_verified": true}); got != "a@example.com" {
		t.Fatal(got)
	}
}
