package goth

import "github.com/not-for-prod/authorization/domain/entity"

func verifiedGoogleEmail(email string, claims map[string]interface{}) string {
	verified, ok := claims["email_verified"].(bool)
	if !ok {
		verified, ok = claims["verified_email"].(bool)
	}
	if !ok || !verified {
		return ""
	}
	return entity.NormalizeVerifiedEmail(email)
}
