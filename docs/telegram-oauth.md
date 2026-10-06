# Telegram OAuth driver

`driver/method/telegram` implements `authorization.CallbackMethod` using Telegram's
[official OIDC authorization-code flow](https://core.telegram.org/bots/telegram-login).
Configure your bot in BotFather's Login Widget settings: register the exact HTTPS
callback URL and obtain the client ID and secret. Keep the default RS256 signing
algorithm. This driver requests only `openid` and validates RS256 tokens.

For local development, the driver also accepts HTTP callbacks on `localhost`
and `127.0.0.1`. Telegram must accept the exact callback URL registered in
BotFather; accepting configuration locally does not guarantee provider approval.

```go
method, err := telegram.New(telegram.Config{
    ClientID:     cfg.TelegramClientID,
    ClientSecret: cfg.TelegramClientSecret,
    RedirectURL:  "https://example.com/auth/telegram/callback",
})
if err != nil {
    return err
}
policy := authorization.Any("primary",
    authorization.Method(method, authorization.WithProviders(telegram.Provider)),
)
// Pass this policy through WithPolicy inside the tenant's WithIntent options.
```

See the compiled [configuration example](../driver/method/telegram/example_test.go).
Reuse the same method pointer across a tenant's policies. The engine allows one
concrete driver per tenant/method ID: this standalone `oauth` driver cannot be
registered alongside a different `oauth` driver in the same tenant. This feature
does not introduce a multi-provider dispatcher.

`Config.Timeout` defaults to ten seconds per HTTP request;
`Config.ChallengeLifetime` defaults to five minutes and is capped by transaction
expiry. Negative durations are rejected. The host's engine operation timeout must
allow token exchange plus signing-key retrieval, and its challenge-size limit must
accommodate the encoded client ID, redirect URL, tenant, state and verifier.
Use the existing engine bounds; a 4 KiB challenge budget fits typical configurations.
The driver uses the wall clock, so keep the host's engine clock synchronized with it.

## Browser start and callback association

1. Establish an unforgeable browser-session association. The host's `Access`
   callback must return the same `Caller.Key` throughout this authentication flow.
2. Call `CreateTransaction`, then `StartMethod` with selection
   `{Requirement: "primary", Method: "oauth", Provider: telegram.Provider}` and
   `Input: telegram.StartInput{}`. Inputs are values; pointer inputs are rejected.
3. After StartMethod returns, parse the `state` query parameter from its redirect
   URL. Store an expiring server-side association keyed by that state **and the
   initiating browser session**: tenant, transaction ID, attempt ID, challenge ID
   and caller association. Keep separate entries for concurrent browser tabs.
4. Redirect the browser to the returned URL only after this association is saved.
   The engine has already persisted the private challenge. Never expose
   `Challenge.State`, the PKCE verifier, or the client secret.
5. At the registered callback route, require the same browser session, bound the
   callback query size, and reject repeated `state`, `code` or `error` parameters.
   Resolve the stored association by state and session. Do not accept tenant,
   attempt, challenge, user or caller authority from query parameters alone.
6. Call `CompleteCallback` with the original IDs from that association and:

   ```go
   telegram.CallbackInput{
       Code:  query.Get("code"),
       State: query.Get("state"),
       Error: query.Get("error"),
   }
   ```

   Restore the trusted caller into the context used by `Access`. The engine checks
   access and the attempt before the driver checks state and exchanges the code.
   `VerifyMethod` can accept the same input when a host uses that entry point.
7. Use the returned engine result/progress. Keep bounded association receipts until
   transaction expiry so callback retries can reach the engine's recorded outcome.
   Suppress query/token logging and redirect away from the callback URL after handling.

## Identity and recovery

The result has identity kind `oauth`, issuer `https://oauth.telegram.org`, and
subject from the verified ID token's `sub`. Profile IDs, phone numbers and usernames
are not used. Existing identities stored under issuer `telegram` are not aliases.
The engine performs existing-user resolution or approved sign-up behavior.

The driver validates signature, issuer, audience, authorized party when relevant,
required expiry/issued-at claims, time bounds and nonempty subject. It fetches the
fixed trusted JWKS endpoint on every successful exchange, supporting key rotation
without a shared cache. No token-provided key URL is followed. Provider responses
are bounded to 64 KiB; redirects and automatic HTTP retries are disabled. Access
and ID tokens never leave the driver.

Wrong correlation, denial, invalid code (`invalid_grant`) and invalid tokens return
`ErrProofRejected`, enabling the existing bounded rejection transition. Transport,
provider/configuration and key-retrieval failures preserve the engine's uncertain
operation fence. A consumed code cannot safely be exchanged again: while claimed,
retries return busy; an expired uncertain claim abandons the transaction, requiring
new authentication. A committed proof instead uses the engine's recorded outcome.
The method alone does not prevent replay; invoke it through the engine.
