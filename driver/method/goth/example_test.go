package goth_test

import (
	"fmt"

	auth "authorization"
	oauth "authorization/driver/method/goth"
)

func ExampleNew() {
	method, err := oauth.New(oauth.Config{
		TenantID:     "demo",
		ClientID:     "google-web-client-id",
		ClientSecret: "host-provided-secret",
		CallbackURL:  "https://app.example/auth/google/callback",
	})
	if err != nil {
		panic(err)
	}
	// Supply this policy with auth.WithPolicy inside the tenant's intent options.
	_ = auth.Any("primary", auth.Method(method, auth.WithProviders(oauth.Google)))
	fmt.Println(method.Name(), oauth.Google)
	// Output: oauth google
}
