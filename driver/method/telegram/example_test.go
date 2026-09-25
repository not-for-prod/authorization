package telegram_test

import (
	"fmt"

	"github.com/not-for-prod/authorization"
	"github.com/not-for-prod/authorization/driver/method/telegram"
)

func ExampleNew() {
	method, err := telegram.New(telegram.Config{
		ClientID:     "123456", // From BotFather; supply the real values through host configuration.
		ClientSecret: "host-provided-secret",
		RedirectURL:  "https://example.com/auth/telegram/callback",
	})
	if err != nil {
		panic(err)
	}
	policy := authorization.Any("primary",
		authorization.Method(method, authorization.WithProviders(telegram.Provider)),
	)
	// Pass this option to authorization.New with the host's dependencies and limits.
	tenant := authorization.WithTenant("example",
		authorization.WithIntent(authorization.IntentSignIn, authorization.WithPolicy(policy)),
	)
	_ = tenant
	fmt.Println(method.Name(), telegram.Provider)
	// Output: oauth telegram
}
