# Telegram OAuth method driver

Status: Approved by the owner on 2026-09-24.

Implement `driver/method/telegram` as a host-configured `CallbackMethod`, selected
with method `oauth` and provider `telegram`. Use Telegram's official OIDC
authorization-code flow with S256 PKCE. The host supplies the BotFather client
ID, client secret, and registered callback URL, and owns callback routing.

Protocol reference: [Telegram Login](https://core.telegram.org/bots/telegram-login).

## Successful authentication

```mermaid
sequenceDiagram
    actor Browser
    participant Host
    participant Engine
    participant Tx as Authentication transaction
    participant Driver as Telegram method driver
    participant Telegram
    Browser->>Host: Start oauth/telegram
    Host->>Engine: StartMethod with typed start input
    Engine->>Tx: Lock, claim pending attempt, commit
    Engine->>Driver: Start outside storage lock
    Driver-->>Engine: Private state and PKCE verifier, expiring challenge, redirect
    Engine->>Tx: Lock, persist challenge and start outcome, commit
    Engine-->>Host: Attempt ID, challenge ID, redirect URL
    Host-->>Browser: Save callback association and redirect
    Browser->>Telegram: Authorize with state and S256 challenge
    Telegram-->>Browser: Redirect to registered host URL with code and state
    Browser->>Host: Callback
    Host->>Engine: CompleteCallback with original caller, transaction, attempt and challenge
    Engine->>Tx: Lock, validate attempt, claim verification, commit
    Engine->>Driver: Callback with stored challenge and typed code/state input
    Driver->>Driver: Validate selection, challenge binding, expiry and state
    Driver->>Telegram: Exchange code using Basic client authentication and PKCE
    Telegram-->>Driver: ID token
    Driver->>Telegram: Fetch signing keys when needed
    Driver->>Driver: Verify signature, issuer, audience, lifetime and subject
    Driver-->>Engine: Verified issuer/subject identity and matching evidence
    Engine->>Tx: Existing subject resolution and atomic proof acceptance
    Note over Engine,Tx: Existing policy and intent finalization; existing session issuance
    Engine-->>Host: Progress or completed authentication
```

## Failure and recovery

```mermaid
sequenceDiagram
    actor Host
    participant Engine
    participant Tx as Authentication transaction
    participant Driver as Telegram method driver
    participant Telegram
    Host->>Engine: CompleteCallback
    Engine->>Tx: Validate and claim original attempt
    alt Expired, mismatched, or already completed attempt
        Engine-->>Host: Existing rejection or recorded outcome; no provider call
    else Pending valid attempt
        Engine->>Driver: Verify callback outside lock
        alt Bad state, malformed input, or user denial
            Driver-->>Engine: Definitive proof rejection, no exchange
            Engine->>Tx: Existing bounded rejection transition
            Engine-->>Host: Authentication rejected
        else Correlated code
            Driver->>Telegram: Exchange code once
            alt Definitive invalid code or invalid ID token
                Driver-->>Engine: Definitive proof rejection
                Engine->>Tx: Existing bounded rejection transition
                Engine-->>Host: Authentication rejected
            else Network ambiguity, provider failure, or key retrieval failure
                Driver-->>Engine: Sanitized error; external result uncertain
                Note over Engine,Tx: Retain existing dispatch fence; no automatic code-exchange retry
                Host->>Engine: Retry original operation
                Engine-->>Host: Busy while claimed; abandoned after expired uncertain claim
                Host->>Engine: Create new authentication transaction and start again
            end
        end
    end
```

If a code was consumed before local acceptance and its result was lost, a fresh
authorization is required. Existing committed outcomes remain replayable through
the engine without exchanging the code again.

## Aggregate boundaries

| Existing root | One-sentence purpose | Impact |
| --- | --- | --- |
| Authentication transaction | Establish one authenticated subject under a fixed tenant and intent policy. | Owns the bounded attempt and private challenge; existing transaction lock, repository, expiry, dispatch fence and atomic event publication apply. |
| User | Represent the canonical account. | Existing sign-in/sign-up behavior; the driver never writes accounts. |
| Identity | Associate a verified issuer and subject with a user. | Existing resolution uses the OIDC issuer and `sub`; no username, phone or email matching. |
| Session | Represent an issued authenticated session. | Existing host-managed issuance after policy satisfaction. |

No new aggregate, repository, migration, or multi-root transaction is introduced.
Challenge state is owned by the authentication transaction because proof acceptance
must consume that same attempt atomically. The driver makes no persistence calls
and never mutates the supplied transaction. Provider calls run outside its lock.
Existing account-creation boundaries remain governed by the approved sign-up model.

## Commands, events and routes

| Existing command | Existing events used by this driver flow |
| --- | --- |
| StartMethod | MethodStartRequested, MethodStarted |
| CompleteCallback / VerifyMethod | MethodVerificationRequested; EvidenceAccepted and optionally PolicySatisfied, or MethodProofRejected |
| Expired uncertain operation recovery | TransactionAbandoned |
| Existing intent finalization | Existing account lifecycle events when applicable, AuthenticationCompleted |

No new event or cross-aggregate event route. Existing events record committed
engine transitions; they do not trigger provider exchanges. Never include codes,
client secrets, PKCE verifiers or tokens in events, errors, logs or evidence.

## Decisions for approval

1. Add the official OIDC browser redirect flow; legacy login-widget signatures,
   Mini App authentication, bot messaging and phone-number permissions are outside
   this feature. Request only `openid`.
2. Return identity kind `oauth`, issuer `https://oauth.telegram.org`, and the
   verified token's `sub`. This intentionally does not alias existing identities
   stored under issuer `telegram` or keyed by the Telegram profile `id`.
3. Both `Verify` and `Callback` use the same code/state proof checks. Start takes
   a driver-owned empty input type; completion takes code, state and optional
   provider error. Redirect URI and credentials come only from host configuration.
4. Use cryptographically random state and PKCE material. Default challenge lifetime
   is five minutes, capped by transaction expiry. Validate transaction and selection
   binding, pending status and expiry before exchanging any code.
5. Use a maintained JOSE implementation for signature and claim verification,
   with trusted Telegram keys and an explicit allowed signing algorithm set.
   Use Resty for direct provider HTTP, bounded responses and timeouts. Do not
   automatically retry token POSTs. Never follow token-supplied key URLs.
6. Host wiring documentation must show how to preserve the original caller and
   transaction/attempt/challenge association across the browser redirect without
   deriving tenant or user authority from untrusted callback parameters.

After approval, derive the implementation artifacts and implement the driver,
host wiring example, deterministic protocol/security tests and repository checks.
