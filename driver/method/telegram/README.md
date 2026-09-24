# Telegram OAuth method

Use `telegram.New(telegram.Config{ClientID: ..., ClientSecret: ..., RedirectURL: ...})`
with method `oauth` and provider `telegram`. The driver implements the official
OIDC code flow with PKCE and RS256 ID-token validation.

See [host configuration, callback association and recovery](../../../docs/telegram-oauth.md)
and the compiled [configuration example](example_test.go).

Run `go test -race ./driver/method/telegram` for the local HTTPS protocol tests.
