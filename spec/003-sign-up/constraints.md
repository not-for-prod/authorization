# Constraints

## Layer ownership
Domain owns shared, intent-independent proof acceptance, binding, and evaluation. Application orchestrates account
creation. Delivery retains the existing interface and response mapping.

## API and protobuf constraints
No changes in this iteration.

## Domain constraints
One private verified identity per transaction. No provisional UserID. UserRegistered
records the committed binding without publishing the identity or credentials.

## Application service constraints
Synchronous completion. Host eligibility outside locks and session calls after
commit. The visual model records the narrow multi-root creation exception under review.

## Repository constraints
Shared atomic transaction for user, identity, transaction and events. Lock the
transaction root first and use unique issuer/subject insertion to resolve races.
No existing user updates. No separate-store registration support.

## Gateway constraints
Methods prove identities. Password enrollment is outside the method proof contract.

## Migration constraints
Host-owned production storage must persist VerifiedIdentity and meet the shared
transaction contract. The supplied in-memory driver copies the whole root.

## Error handling constraints
Classify invalid proof, subject mismatch and identity conflict distinctly. Storage
failure must not be interpreted as an absent identity.

## Tracing and observability constraints
Retain application entry spans and avoid identity/credential event payloads.

## Generation constraints
No interface/proto generator inputs changed by this iteration.
