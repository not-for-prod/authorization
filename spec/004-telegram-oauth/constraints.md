# Layer ownership
All new behavior belongs to driver/method/telegram; host owns routes and credentials.
# API and protobuf constraints
No changes. Export driver-owned typed inputs.
# Domain constraints
No new roots; existing transaction owns bounded challenge and proof acceptance.
# Application service constraints
Reuse existing replay, claims, identity resolution and finalization.
# Repository constraints
No new storage or direct persistence.
# Gateway constraints
Use Resty for direct HTTP and go-jose/v4 for RS256/JWKS verification. Fixed Telegram endpoints, no redirects, automatic retries, token URL key lookup or debug logs. Fetch trusted JWKS on each successful token exchange so rotation needs no new persistent/cache state.
# Migration constraints
None needed.
# Error handling constraints
Definitive bad proof wraps gateway.ErrProofRejected and proterror.Unauthenticated; malformed local input uses InvalidArgument with proof rejection. Uncertain provider/network errors exclude ErrProofRejected and retain dispatch fence. Sanitize low-level provider errors before tracing.
# Tracing and observability constraints
Use prospan at context-aware boundaries; omit all credential/proof attributes.
# Generation constraints
No generated files change.
