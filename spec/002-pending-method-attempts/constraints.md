# Layer ownership
Domain owns attempts, transitions and event types. Application loads, invokes, saves and publishes. Drivers run outside locks.
# API and protobuf constraints
Single service.proto, additive fields, required attempt ID for Verify and Callback.
# Domain constraints
One transaction root owns bounded attempts, claims, evidence and events. Pure evaluator passed to root through a domain interface.
# Application service constraints
Explicit public flows, DTO Verify, direct field access, synchronous completion.
# Repository constraints
One shared transaction manager for state and outbox, row locking, rollback together.
# Gateway constraints
Existing driver interfaces remain. No proof or token in events.
# Migration constraints
No database added to ephemeral test host. Durable production publication uses Vibox with host database, outside this slice.
# Error handling constraints
Missing subjects stay NotFound, wrong proof Unauthenticated, invalid attempt FailedPrecondition.
# Tracing and observability constraints
Preserve spans, never trace credentials.
# Generation constraints
Run make pb and Go generation only where needed.
