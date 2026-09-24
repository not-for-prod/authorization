# Acceptance criteria
Only a valid correlated callback carrying a Telegram-signed unexpired RS256 token for the configured client produces identity and matching evidence. No local invalid callback reaches the provider. Uncertain exchanges are never automatically repeated.
# Success scenarios
Start produces independent random state/verifier values and correct S256 redirect. Valid Verify and Callback return issuer/sub evidence. Key retrieval uses the fixed trusted JWKS endpoint.
# Failure scenarios
Wrong type, nil pointer, selection/tenant/challenge mismatch, expired challenge, denial, bad state, bad code, invalid signature/algorithm/claims, malformed or oversized responses, HTTP errors, context cancellation and redirects fail closed. Secrets never appear in returned errors.
# Regression checks
Existing Go tests remain passing; no core or generated contract changes.
# Required tests
Deterministic local HTTP server tests for request construction, Basic authentication, PKCE, claim validation, hostile keys, error classification, no retry, response limits, redirects and concurrent calls; compiled host configuration example.
