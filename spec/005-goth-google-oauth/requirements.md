# Requirements
## Problem
There is no real Google method driver. Verification currently calls providers while holding storage locks and rolls back dispatch claims on failure.
## Goals
Authenticate Google subjects through Goth and fence ambiguous authorization-code exchanges.
## Non-goals
Other providers, account linking, refresh tokens, transport routes, new aggregates or migrations.
## Functional requirements
1. Register method oauth/provider google per tenant using host credentials and a fixed callback URL.
2. Start returns a redirect and private random-state challenge capped by transaction expiry.
3. Callback and Verify validate typed input, state, selection, tenant/transaction association and expiry before exchanging a code.
4. Return oauth identity with issuer https://accounts.google.com and authenticated Google subject. Never use email for identity resolution.
5. Persist exclusive claims before verification I/O, accept evidence and consume challenge atomically after reloading the root, and reject duplicate completed attempts.
6. Definitive rejection releases the claim under attempt limits. Uncertain outcomes retain it. A mutating retry abandons an expired claim and requires a new transaction.
## Data and contract requirements
Driver-owned Config, StartInput and CallbackInput. Existing gateway and protobuf contracts suffice. Store only pre-authorization Goth session and correlation metadata in challenge state; never persist provider tokens.
## Validation requirements
Reject invalid configuration, unexpected or nil input, missing state/code, denied consent, expired or mismatched challenges and empty subjects. Bound network time and response size; sanitize upstream errors.
## Compatibility requirements
Existing Go API and protobuf shapes unchanged. Verification error paths now commit dispatch/rejection events and claims as approved. Host persists browser/caller correlation and passes existing attempt identifiers.
## Open questions
None blocking. Confidential-client flow without PKCE follows the approved model.
