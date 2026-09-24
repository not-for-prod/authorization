---
name: go-code-style
description: Resolve Firestarter-specific Go implementation-style questions using the repository lint contract and the Uber Go Style Guide. Use for focused style reviews or ambiguous choices involving interfaces, initialization, concurrency, map or slice ownership, imports, or table-driven tests; use `naming` for identifier work and `function-decomposition` for function shape.
---

# Go Code Style

## Source

Follow the official Uber Go Style Guide:

- https://github.com/uber-go/guide/blob/master/style.md

Use this skill as the local checklist. If a question is subtle or not covered here, consult the official guide and prefer local repository conventions when they are stricter.

## Baseline

- Always run `gofmt` on edited Go files.
- Prefer `goimports` when import grouping or unused imports are involved.
- Keep code simple, explicit, and idiomatic.
- Match the surrounding package style unless it conflicts with this guide or project skills.
- Write the smallest code shape that fully solves the task.
- Prefer direct readability over cleverness, speculative extension points, or imaginary future performance.
- Do not add argument or field validation as a general code-style reflex. Validation placement is owned by the `field-validation` skill: validate field shape at first occurrence only, and do not repeat it through every layer.

## Interfaces

- Do not use pointers to interfaces.
- Accept interfaces and return concrete types when that keeps ownership clear.
- Verify interface compliance when a type is meant to satisfy an important contract:

```go
var _ repository.Outbox = (*Implementation)(nil)
```

## Errors

- Handle each error once.
- Wrap or classify errors at the lowest meaningful layer according to project error skills.
- Do not panic for normal error paths.
- Error strings should be lower-case and should not end with punctuation.
- Prefer specific error context over generic messages.

## Data Boundaries

- Copy slices, maps, and mutable byte-like values when crossing ownership boundaries.
- Do not expose internal mutable state from entities, DTOs, caches, or config.
- Use field tags only on structs that are actually marshaled, scanned, or bound by a tool using those tags.

## Concurrency

- Use `defer` for cleanup such as unlocks and closing resources.
- Do not fire-and-forget goroutines. Make lifecycle, cancellation, and error handling explicit.
- Channels should usually be unbuffered or size one; larger buffers need a concrete reason.
- Avoid mutable globals.
- Avoid `init()` unless there is no clearer explicit wiring path.

## Names And Layout

- For variable, function, type, package, file, DTO, and helper names, apply `naming`.
- Avoid names that shadow built-ins.
- Keep import groups ordered: standard library, third-party, local project.
- Use import aliases only when they improve clarity or avoid a real conflict.
- Group related declarations, but keep method-per-file notation when that skill applies.
- Reduce nesting with early returns.
- Do not use unnecessary `else` after a returning branch.
- When selecting distinct flows by an enum value, prefer a `switch` with named cases for each supported flow instead of treating every other value as one flow through `else`. Keep simple single-condition guards as `if` statements.
- For helper extraction, function splitting, or abstraction decisions, apply `function-decomposition`.

## Blank Lines Inside Functions

- Apply the same spacing convention across all layers, including private helpers and test fixtures.
- Separate logical steps with one blank line: setup, validation, dependency calls, state changes, and response construction.
- Keep an assignment or call together with its immediate error check; add a blank line after the check before continuing to the next step.
- Separate consecutive guard clauses and the statements following a completed control-flow block. Keep related assignments together rather than spacing every statement.
- Keep resource acquisition next to its `defer`; separate that setup from subsequent work. Do not add blank lines immediately inside opening or closing braces.
- Review spacing explicitly: `gofmt` does not insert missing logical separators.
- When wiring pseudocode or copying a sibling method, preserve its logical blank-line grouping and audit every touched sibling before handing it back. Compilation and formatting success do not substitute for this check.

## Direct Field Access

Use DTO fields and cheap, pure getters at the point of use. Do not introduce
locals that merely alias them, even when used more than once, unless the local
captures a meaningful snapshot, an expensive result, or a value that is actually
transformed. Prefer the authoritative stored field directly when a flow switches
from request data to persisted state. Audit adjacent methods when correcting
this pattern; do not stop at the highlighted lines.

## Initialization

- Use field names when initializing structs unless the type is a tiny local value where positional initialization is obviously clearer.
- Omit zero-value fields from struct literals unless the zero value carries important meaning.
- Prefer `var value T` for zero-value structs.
- Prefer specifying capacity for slices and maps when the size is known.

## Performance Hygiene

- Prefer `strconv` over `fmt` for simple numeric/string conversions.
- Avoid repeated string/byte conversions in hot or repeated paths.
- Do not optimize at the cost of clarity unless there is a measured reason.
- Use maps, sets, sorting, batching, or concurrency when the data shape or task justifies them, not as default ornamentation.

## Tests

- Prefer table-driven tests for multiple cases.
- Keep test case names descriptive.
- Test behavior and boundary conditions, not implementation details.

## Review Checklist

Before finalizing Go changes, check:

- edited files are formatted
- errors are classified/wrapped consistently with project conventions
- no pointer-to-interface, avoidable globals, or unnecessary `init()`
- mutable boundary data is copied where ownership changes
- imports are clear
- names follow `naming`
- function decomposition decisions follow `function-decomposition`
- DTO tags are present only when used
- tests or compile checks cover the touched package
