---
name: function-decomposition
description: Prevent over-decomposition and decide when a function may be split, moved, or abstracted. Use whenever Codex considers extracting a helper or callback, reviews a function whose cognitive complexity is greater than 20 or whose length is greater than 60 lines, or proposes an extraction to remove proven code duplication.
---

# Function Decomposition

Keep readable behavior in one place. Unnecessary helpers force readers to jump between functions and hide the business and domain-event flow.

## Hard Gate

Do not decompose a function or method unless at least one condition is true:

- cognitive complexity is greater than 20
- the function body is longer than 60 lines
- the extraction removes proven existing code duplication

Treat both thresholds as strict `>` comparisons. Cognitive complexity 20 and exactly 60 lines stay inline.

Passing the gate permits decomposition; it does not require it. Keep even a longer straight-line orchestration function inline when splitting it would make the complete flow harder to follow.

Before extracting helpers to shorten a use case, remove redundant branches and consolidate equivalent fresh-request and replay response paths. Judge simplification by the complete flow and total code, not only the public method length; preserve the distinction between safe pre-dispatch failures and uncertain external outcomes.

For executable examples, keep dependency construction, service configuration, and server startup together in `main` when the user wants a linear setup. Do not extract the composition root solely to make it callable from a unit test; supporting adapter implementations may remain separate.

Existing interface methods, framework callbacks, and functions required by generated contracts are not optional helper extraction and are not governed by this gate.

DTO-owned optional-field access and field validation are also outside this gate.
Move pointer dereferencing with a zero/default fallback into a typed DTO getter
such as `GetChallengeID()`, and request field checks into `Verify() error` as
specified by field-validation. Keep these methods beside the DTO rather than
adding service helpers or separate method files. Getters must not mutate the DTO
or turn an explicitly invalid value into a valid one; validation owns that check.

## Default

Keep code inline. Below the hard gate, do not extract:

- shared read-modify-write transaction helpers
- one-use helpers
- helpers that only shorten the caller
- helpers for a small repeated block without proven duplication
- callback-based transition wrappers
- speculative reusable abstractions
- helpers added merely to name a few obvious lines

Prefer a small amount of explicit duplication over hiding a short business flow.

For an aggregate command, keep this sequence visible in the public method:

1. load and lock the aggregate
2. call the aggregate method and receive `domainEvent`
3. return on error or event-free no-op
4. persist the aggregate
5. publish that exact domain event

Do not replace that visible sequence with a generic `update`, `mutate`, `transition`, or `do` helper when the method is below the gate.

## Duplication Exception

Allow decomposition below both thresholds only when all of these facts are shown before editing:

- at least two existing production call sites contain the same meaningful behavior
- the duplicated statements and branches are identified by file and function
- the proposed extraction removes duplicated production code in the resulting diff
- the helper has one precise responsibility and an intent-revealing name
- the extraction does not hide a business workflow, aggregate transition, persistence step, or domain-event publication

Use repository search and the before/after diff as proof. Similar-looking code, possible future reuse, a one-use helper, duplicated tests, or fewer caller lines alone are not proof.

State the proof in the work update or final response: name the call sites and what duplicated behavior the extraction removed. If that evidence cannot be stated concretely, keep the code inline.

Apply the same gate in reverse during review: remove single-use pass-through helpers that only rename one call. Shared technical primitives may use the duplication exception when repository search proves at least two production copies—for example, identical event-context mapping or bounded response reading—but the extracted primitive must not conceal an aggregate transition or use-case step.

## Application Call Chains

When reviewing orchestration, trace the full public-to-private call chain, not
only individual function length. Public use cases should show their distinct
external operation, subject resolution, persistence phase, and completion choice.
Avoid a private dispatcher that accepts operation-kind flags and branches back
into the public use cases it hides. Keep truly shared atomic persistence or
recovery behavior in one clearly named helper, and call it directly from each
use case. Do not replace one deep chain with another layer of pass-through helpers. Intent selection is also a use-case decision: keep the
intent branch visible in each public entrypoint and call the shared persistence
phase directly. Two callers do not justify a private dispatcher whose main job
is choosing an intent and forwarding to other helpers.

Keep short access-hook calls and their caller/tenant checks visible in public
entrypoints. Do not wrap operation acquisition in a private application helper
that hides loading, locking, replay/recovery decisions, persistence, and event
publication behind an operation-kind argument. Inline and specialize that
orchestration in each entrypoint; retain domain methods for actual invariants.
Repeated callers alone do not outweigh the navigation cost of these wrappers.

When the owner explicitly asks to postpone decomposition, inline the existing
private application call chains across all affected entrypoints. Do not replace
them with local closures or new wrappers. Keep only callbacks required by external
contracts and existing domain/dependency boundaries; let the owner choose later
extractions even when the inline methods exceed the usual thresholds.

## Review After The Gate

When a function passes the gate, first classify the source of complexity:

- domain policy belongs on the aggregate or value object
- provider, SQL, transport, signing, or conversion mechanics belong at their owning boundary
- multiple real phases may become focused functions
- straight-line orchestration may remain inline

Extract only when the result is easier to understand end to end. Every helper must name a real concept or ownership boundary. Reject vague helpers such as `update`, `process`, `prepareData`, `handleResult`, or `do`.

## Measuring

- Count the function body from its opening brace through its closing brace; exclude comments and blank lines.
- Use the repository linter's cognitive-complexity result when available.
- If no tool reports cognitive complexity, conservatively keep the function inline unless its structure clearly exceeds 20.
- Do not estimate a high score merely to justify a preferred extraction.

## Final Check

- Did every new extraction pass cognitive complexity `> 20`, length `> 60`, or the complete proven-duplication exception?
- If using the duplication exception, are at least two original production call sites and the removed duplicated behavior documented?
- Can the public use case be understood without navigating to a trivial helper?
- Is the aggregate-returned domain event visible before persistence and publication?
- Is any remaining decomposition based on a real phase or ownership boundary?
