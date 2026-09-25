# Problem
Only process-local storage drivers exist.
## Goals
Provide durable PostgreSQL storage and shared Redis sessions through existing ports.
## Non-goals
No business flow, policy, protobuf, broker, or deployment changes. No standalone Session repository.
## Functional requirements
All PostgreSQL mutations use the same manager context. Existing roots require a preceding locked read. Sign-up commits account creation, binding, and events together. Redis Create is atomic and idempotent by tenant/transaction, Validate rejects expired/revoked credentials, and Revoke is idempotent.
## Data and contract requirements
Persist every transaction field except unpublished events. Preserve interface-valued policy trees through an explicit versioned codec. Enforce global identity subject uniqueness. Retain Redis issuance records through max(session expiry, recovery deadline), without reactivating revoked credentials.
## Validation requirements
Reject unsupported select options, mutations without a transaction, nested/foreign/closed scopes, immutable changes and mismatched issuance principals. Preserve not-found/duplicate sentinels and classifications. Tenant and owner predicates are exact.
## Compatibility requirements
Keep current ports and in-memory drivers. Hosts own connections and tenant configuration. Use additive migrations.
## Open questions
None affecting approved behavior.
