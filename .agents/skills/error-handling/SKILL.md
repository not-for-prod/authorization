---
name: error-handling
description: Apply Firestarter's `proterror` contract to classify, wrap, and propagate Go errors at their origin. Use when implementing or reviewing repository, gateway, application, or delivery error paths; choosing typed classifications; preventing double-wrapping; or coordinating error ownership with `span.Err(...)` tracing.
---

# Error Handling

## Overview

Apply structured error handling in existing Go code by classifying errors with `proterror` at the layer where the original error occurs. Reuse tracing when present, so origin returns become `span.Err(errors.Join(&proterror.X{}, err))`. Higher layers must not add another `errors.Join`, `fmt.Errorf`, or `proterror` around an already-classified error, though they may return `span.Err(err)` to record the failure on their own span when local tracing conventions do that.

Use typed errors from:

```go
import "github.com/not-for-prod/proterror/proterror"
```

## Workflow

Follow this sequence:

1. Inspect each error return in the target methods.
2. Decide whether the current method is the lowest level where the error first occurs.
3. Choose the most specific `proterror` kind.
4. Replace plain returns with `errors.Join(...)`.
5. If tracing is already present, wrap the classified error with `span.Err(...)`.
6. If extra message context is needed, add `fmt.Errorf("...: %w", err)` only at that same lowest layer before `errors.Join(...)`.
7. Do not reclassify errors that should already have been classified in deeper methods.

## Main Rule

At the lowest stack level where the error first occurs, wrap the original error with the appropriate `proterror` type.

Without tracing:

```go
if err != nil {
	return errors.Join(&proterror.NotFound{}, err)
}
```

With tracing:

```go
if err != nil {
	return span.Err(errors.Join(&proterror.NotFound{}, err))
}
```

For multi-return methods:

```go
if err != nil {
	return nil, span.Err(errors.Join(&proterror.Internal{}, err))
}
```

## Lowest-Level Ownership

Classify the error where it first occurs.

Example stack:

- delivery
- application
- repository

If the repository receives the original database error, the repository owns:

1. choosing the correct `proterror` type
2. wrapping the original error with `errors.Join(...)`
3. wrapping the result with `span.Err(...)` if tracing is present

Higher layers must not classify the same error again unless they create a genuinely new local error at that layer. They also must not add a second `fmt.Errorf(... %w ...)` around an error that should already have been wrapped and classified deeper in the stack.

## No Double-Wrapping

PostgreSQL adapters must classify original database errors with
`proterrors.FromPG(err)` from `github.com/not-for-prod/proterror` (the package
name is `proterrors`), using `span.Err(proterrors.FromPG(err))` when traced.
Preserve repository sentinels required by the port alongside that classification.
Non-PostgreSQL adapters classify their own storage failures at the source.
Do not add application-level sentinel switches or `errors.As` fallback
classifiers to compensate for repository errors: fix the adapter and propagate
its error unchanged. Translate domain errors only at their first application
call site, using the classifications that operation can actually produce.

If a lower layer already returned a classified error, do not classify it again:

```go
if err != nil {
	return err
}
```

In a traced public method, this is also acceptable when the current span should record the failure:

```go
if err != nil {
	return span.Err(err)
}
```

Do not do this:

```go
if err != nil {
	return span.Err(errors.Join(&proterror.Internal{}, err))
}
```

unless the current method is the first place where that error originated.

The same rule applies to `fmt.Errorf`: do not add more context in higher layers just because the error is passing through them. Add message context only where the original error is first being turned into the returned error for that stack path.

## Classification Rules

Choose the narrowest matching `proterror` kind:

- missing record, no rows, entity not found: `&proterror.NotFound{}`
- duplicate key, already exists: `&proterror.AlreadyExists{}`
- invalid parse, bad input, validation failure: `&proterror.InvalidArgument{}`
- forbidden action, permission failure: `&proterror.PermissionDenied{}`
- illegal state, failed precondition: `&proterror.FailedPrecondition{}`
- missing or invalid auth: `&proterror.Unauthenticated{}`
- temporary downstream outage: `&proterror.Unavailable{}`
- unknown or infrastructure failure: `&proterror.Internal{}`

Prefer the most specific type over `Internal`.

Preserve missing-user and missing-identity lookup errors as `NotFound` during
authentication resolution. When verified proof has no registered account, preserve a
distinct application sentinel alongside `NotFound` so hosts can offer registration
without mistaking unrelated missing records for an unknown account. Do not replace a storage miss with invalid proof or
`Unauthenticated`; invalid credentials and proofs are separate failures.

### Domain errors

Domain entities may return domain-specific sentinel or typed errors without importing `proterror`. The application layer owns the first translation from a known domain failure to the public classification required by the use case, for example joining an invalid transition with `FailedPrecondition`. Preserve the domain error in the chain so `errors.Is` and `errors.As` still work.

Do not classify the same domain error again in delivery. Unknown errors returned from deeper layers must not be flattened to `Internal` merely because delivery does not recognize them.

## Tracing Interaction

If the method already has tracing:

```go
ctx, span := prospan.Start(ctx)
defer span.End()
```

then lowest-level returns should look like:

```go
if err != nil {
	return span.Err(errors.Join(&proterror.NotFound{}, err))
}
```

or:

```go
if err != nil {
	return nil, span.Err(errors.Join(&proterror.Internal{}, err))
}
```

If tracing is not present, still classify the error with `errors.Join(...)`.

## New Errors At Current Layer

If the current method creates a new error locally, classify that new error at this layer. If you need `fmt.Errorf`, use it only here, before the first `errors.Join(...)` for that error:

```go
if err != nil {
	return span.Err(errors.Join(&proterror.Internal{}, fmt.Errorf("build response: %w", err)))
}
```

A successful dependency lookup can produce a new application-owned error, such
as an already-existing resource blocking creation. Classify that error at the
branch that creates it: `errors.Join(&proterror.AlreadyExists{}, repository.ErrAlreadyExists)`.
Returning a repository sentinel from application code does not make the error
repository-owned; preserve the sentinel and add its public classification here.

## Transformation Rules

When applying this skill:

- inspect each error return
- determine whether the current method is the lowest level where the error first occurs
- determine the most specific `proterror` type
- if message context is needed, add `fmt.Errorf("...: %w", err)` only at the same lowest layer where the error is first joined
- replace plain returns with classified errors using `errors.Join(...)`
- if tracing is present, wrap the classified error with `span.Err(...)`
- do not reclassify errors owned by deeper context-aware methods
- use `span.Err(err)` in higher traced methods only to record the existing error, not to add a new classification
- preserve signatures and return semantics

## Constraints

- No double-wrapping already classified errors
- Use `fmt.Errorf("...: %w", err)` only at the lowest stack layer where the error is first joined and classified
- No business-logic changes
- Prefer the most specific `proterror` type over `Internal`
- Preserve existing method signatures
- Prefer minimal diffs
