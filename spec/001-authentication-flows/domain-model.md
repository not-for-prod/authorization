# Domain model
Status: Derived from approved model.

See the approved Mermaid diagrams and aggregate table in
../../docs/event-storming/authentication-flows.md.

Authentication transaction: ID generated once, unique tenant/ID storage key.
Owns policy snapshot, caller key, optional user ID, evidence, bounded challenges,
claim token/deadline, final principal and completion operation. Excludes user
lifecycle, identity ownership, session tokens, provider inputs, and broker state.
Only its repository writes that state; every update holds its root lock.

Transitions: create active; claim attempt; attach challenge or accept evidence;
reject and release a definitive failed attempt; abandon an uncertain expired
claim; prepare final principal; complete. No-op retries do not accept more evidence.

User/Identity are existing records accessed read-only. Session manager accepts
CreateSessionRequest keyed by tenant and authentication transaction ID, and
rejects key reuse for a different principal. The same credential is recoverable
until the authentication transaction expires. No cross-root atomicity is claimed.
