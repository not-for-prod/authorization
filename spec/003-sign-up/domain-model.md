# Boundaries

See the root table and transaction exception in the [visual model](../../docs/event-storming/sign-up.md).
Transaction owns the shared verified identity and optional resolved UserID for both intents. User and Identity remain distinct
persisted roots. This initial synchronous implementation creates them together
with final binding to prevent partial registration. It neither modifies existing
users nor introduces a background workflow. Session issuance remains outside locks.

The owner requested implementation-first correction. Do not generalize this
creation exception to updates or delete operations without a new design decision.
