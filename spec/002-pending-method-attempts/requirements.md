# Problem
Verification currently permits proofs without a pending started method.
# Goals
Transaction-owned pending attempts and policy transitions.
# Non-goals
Registration, enrollment, asynchronous finalization and deployment infrastructure.
# Functional requirements
Start reserves and starts one attempt. Verify and Callback match its ID and selection. Invalid proof permits bounded retry. Identical successful requests reuse stored outcomes. Policy evaluation and completion intent belong to the root.
# Data and contract requirements
Return attempt ID, pending progress and status. Require attempt ID for Verify and Callback. Emit versioned immutable before/after events for every transition. Commit root and outbox together.
# Validation requirements
DTO Verify methods own required fields. Root owns pending selection, claims, expiry and retry checks.
# Compatibility requirements
Direct verification becomes invalid. Hosts supply an event publisher sharing storage transactions. Update examples and bindings.
# Open questions
None for the approved in-memory slice. Production durability remains host integration using the publisher port and Vibox.
