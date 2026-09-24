# Requirements
Status: Derived from approved model; reconciled with prompt/00 on 2026-09-23.

## Problem
The contracts and evaluator cannot yet execute authentication through an API.

## Goals
Run the approved existing-user flows with injected host dependencies.

## Non-goals
Registration, account linking, tenant route changes, a standalone server, live
providers, a database adapter and event publication are outside this slice.

## Functional requirements
- Bind every authentication transaction to an authorized caller, tenant, intent,
  policy snapshot, limits, and expiry. Resolve configured methods per tenant.
- Allow direct proofs, bounded challenges, callbacks, MFA, principal-only and
  session-bearing completion. Resolve only existing eligible users by verified
  user ID or issuer/subject; never merge on email.
- Persist a unique bounded claim before external method calls. Keep remote work
  outside locks; reject concurrent/replayed calls and stale results.
- Commit evidence, user binding, and challenge consumption through one root.
- Definitive rejection allows bounded retries; unknown outcomes require a fresh
  authentication transaction after the claim deadline.
- Persist final principal before idempotent session issuance. Recover lost
  completion responses without repeating proof verification or storing tokens.
- Validate transport requests and driver output, redact internal errors, and
  expose only the existing protobuf response fields.

## Data and contract requirements
Preserve authorization.v1. Snapshot policy/settings/caller identity on the root.
Own challenges in the transaction repository. Add a stable session issuance key.

## Validation requirements
Use protovalidate at delivery; embedded callers have application validation.
Require host access and eligibility rules, validate driver results and enforce
root lifecycle/subject/selection/expiry invariants.

## Compatibility requirements
Public API stays unchanged. Internal gateway adapters must adopt idempotent
CreateSessionRequest; challenge writes move to the transaction repository.

## Open questions
None blocking within the approved scope.
