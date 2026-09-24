# Requirements

## Problem
Unknown identities cannot complete registration through existing-user resolution.

## Goals
Restore existing flows and implement identity-based SIGN_UP for owner correction.

## Non-goals
Password or MFA enrollment, profile attributes, identity linking, UPDATE and DELETE.

## Functional requirements
- Preserve Start, Verify and Callback contracts and caller/tenant guards.
- Both intents retain verified kind/issuer/subject and resolve an existing UserID when available.
- Proofs must concern the same unresolved identity or the same resolved user.
- Apply intent-specific account-existence rules only at finalization.
- Create one active user and initial identity only after all requirements pass.
- At SIGN_UP finalization, reject owned identities, including concurrent competing registrations.
- Roll back all account writes on persistence/publication failure.
- Preserve optional session issuance and retry the same completion after failure.

## Data and contract requirements
One private verified identity on Transaction, deep-copied and excluded from public
progress and event snapshots. Existing protobuf/application interfaces are unchanged.
All participating repositories must share the same storage transaction.

## Validation requirements
Reject authenticated sign-up callers, malformed proof, changed subjects and host
eligibility failures. Keep proof/claim expiration and policy bounds.

## Compatibility requirements
Restore existing application behavior and keep the four intent names. New hosts
using SIGN_UP must support atomic User/Identity writes with Transaction and Events.

## Open questions
Initial active status, one initial identity, and the multi-root creation exception
are explicit first-version decisions for owner correction, not settled production policy.
