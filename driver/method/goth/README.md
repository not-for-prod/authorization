# Google OAuth with Goth

`goth.New(config)` implements `oauth/google` through Goth v1.82.0. It supports
both `CompleteCallback` and challenge-based `VerifyMethod`. Each instance is
bound to one tenant and owns its Google client configuration. It uses no global
Goth registry or Gothic cookie store.

## Configure Google and the host

Create a Google OAuth **Web application** client and register the exact callback
URL used by the host. Configure the consent screen and test users as required by
Google. See [Google's web-server setup](https://developers.google.com/identity/protocols/oauth2/web-server).
Keep the client secret in host configuration.

```go
import (
    auth "authorization"
    gothmethod "authorization/driver/method/goth"
)

method, err := gothmethod.New(gothmethod.Config{
    TenantID:     "demo",
    ClientID:     googleClientID,
    ClientSecret: googleClientSecret,
    CallbackURL:  "https://app.example/auth/google/callback",
})
if err != nil {
    return err
}

// Pass the host's repositories, transaction manager, event publisher, access
// and eligibility hooks as deps. All persistence ports must share the manager.
engine, err := auth.New(deps,
    auth.WithLimits(limits),
    auth.WithTenant("demo",
        auth.WithIntent(auth.IntentSignIn,
            auth.WithPolicy(auth.Any("primary",
                auth.Method(method, auth.WithProviders(gothmethod.Google)),
            )),
        ),
        auth.WithIntent(auth.IntentSignUp,
            auth.WithPolicy(auth.Any("primary",
                auth.Method(method, auth.WithProviders(gothmethod.Google)),
            )),
        ),
    ),
)
```

Use a separate driver instance for each tenant/client. `CallbackURL` requires
HTTPS, except for HTTP localhost/loopback during development. Zero
`ChallengeLifetime` means five minutes, capped by the authentication transaction's
expiry. Zero `Timeout` means ten seconds for the complete provider operation.
`Now` defaults to `time.Now`; supply the same clock as the engine in tests.
`HTTPClient` is optional. Its transport must be concurrency-safe, must not retry
code exchanges and must not log secrets. The driver copies client settings,
disables cookies/redirect following, and propagates callback cancellation.

Session issuance is optional and uses the host's engine session manager, never a
Google access token. To issue a session, configure `WithSessionManager` on the
tenant and `WithSessionIssuance` on the intended flows.

## Start and complete an attempt

After `CreateTransaction`, call:

```go
started, err := engine.StartMethod(ctx, auth.StartMethodRequest{
    TenantID:      "demo",
    TransactionID: transactionID,
    Selection: auth.MethodSelection{
        Requirement: "primary", Method: "oauth", Provider: gothmethod.Google,
    },
    Input: gothmethod.StartInput{},
})
```

The host saves the returned transaction, attempt and challenge identifiers in its
browser-associated session, then redirects to `started.RedirectURL`. Preserve
`Caller.Key` across this round trip. Correlate multiple simultaneous flows by the
returned redirect's `state` using host session storage. Never expose
`Challenge.State`, accept a caller key from the callback query, or let the callback
select the tenant's Google credentials.

On the registered host callback route, reject duplicate `state`, `code`, and
`error` parameters. Recover the original association from trusted browser/session
state and construct:

```go
completed, err := engine.CompleteCallback(ctx, auth.CompleteCallbackRequest{
    TenantID:      associatedTenantID,
    TransactionID: associatedTransactionID,
    AttemptID:     associatedAttemptID,
    ChallengeID:   associatedChallengeID,
    Input: gothmethod.CallbackInput{
        Code:  query.Get("code"),
        State: query.Get("state"),
        Error: query.Get("error"),
    },
})
```

Typed values and non-nil pointers are accepted. For the optional protobuf/HTTP
adapter, implement the host input decoder using the same driver types. No Google
callback route or automatic input decoder is installed by this package.

The driver validates state before exchanging a code. It fetches authenticated
Google userinfo and returns identity kind `oauth`, issuer
`https://accounts.google.com`, and Google's stable user ID as subject. Email is
never used to find or merge accounts. Sign-in needs an existing identity; sign-up
uses the engine's existing new-account and conflict rules.

## Failure behavior

- Invalid input/state, denied consent, invalid grants and empty subjects fail
  without producing evidence. Definitive rejection is identifiable through
  `errors.Is(err, auth.ErrProofRejected)` and consumes an engine attempt.
- Callback and Verify commit an exclusive operation claim before provider calls.
  Concurrent requests are rejected as busy. Successful proof consumes the
  challenge atomically, clears private state and rejects duplicate completion.
- Transport failures, cancellation, malformed provider responses or failed
  acceptance persistence leave the claim fenced. Do not automatically repeat the
  code exchange. A retry after claim expiry abandons the authentication
  transaction; start a new transaction and Google authorization flow.
- A lost successful response can be inspected with `GetTransaction`. This driver
  does not provide recovery of a raw engine session token.

Provider responses are limited to 1 MiB. Provider errors are sanitized before
returning or tracing. Access, refresh and ID tokens are transient and are not
persisted or returned. Goth's userinfo token query is converted to a bearer
header. The driver uses the approved confidential-client flow; Goth's released
Google session does not send PKCE. It does not implement token refresh, account
linking, standalone ID-token verification, or additional providers.

Run deterministic protocol and engine integration tests without Google credentials:

```sh
go test -race ./driver/method/goth
```
