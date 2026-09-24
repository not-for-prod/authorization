# Evaluating authentication evidence

The zero-value [Evaluator](../domain/service/evaluator.go) implements
policy evaluation without storage or provider dependencies:

```go
evaluator := service.Evaluator{}
decision, err := evaluator.Evaluate(ctx, transaction, policy)
if err != nil {
    return err
}
if decision.Satisfied {
    // Continue the flow after validating its user binding and lifecycle state.
} else {
    // Use decision.Remaining to discover eligible method selections.
}
```

The caller supplies verified evidence. Evaluation does not verify credentials,
resolve identities, enforce transaction expiry, check enrollment, or create a
session. A satisfied policy is one prerequisite for completing authentication;
the engine must still enforce those other flow requirements.

## Matching and remaining requirements

A method leaf uses the nearest enclosing nonempty requirement ID as its evidence
scope. Structural groups can omit IDs. Named IDs must be unique throughout a
policy, and every method leaf must have a scope. For example, an anonymous outer
`all` can contain `primary` and `mfa` groups, each with its own alternatives.

Evidence matches the exact requirement ID and method. A provider allowlist
requires one of the listed providers; a method with no providers requires
providerless evidence. Empty provider entries are invalid. Duplicate evidence
has no additional effect, and unrelated evidence does not satisfy a requirement.
Evidence scoped to `primary` never implicitly satisfies `mfa`.

`any` succeeds when at least one child succeeds. `all` succeeds when every child
succeeds. Evaluation examines every branch, even after finding a successful
alternative, so a malformed policy cannot be accepted through short-circuiting.

Satisfied branches are removed. Anonymous groups with one remaining child are
collapsed; named groups retain their IDs and operators to preserve evidence
scope. Thus satisfying `primary` in `all(primary, mfa)` leaves the named `mfa`
group. When everything is satisfied, the result is `Satisfied: true` with
`Remaining: nil`.

The remaining tree is an independent copy. Its groups use value nodes even if
the input used pointers. Updating its child or provider slices cannot change the
original policy. It can be evaluated again with the same or additional evidence.

## Invalid inputs

Errors preserve identity through `errors.Is`:

- `service.ErrPolicyMismatch`: transaction tenant or intent differs from the policy.
- `service.ErrInvalidPolicy`: missing policy tenant/intent, nil or unsupported
  nodes, empty groups, unscoped or unnamed methods, duplicate requirement IDs,
  invalid provider entries, or nesting beyond `MaxRequirementDepth` (64 nodes
  along one path). The depth bound also rejects cyclic graphs safely.
- Context cancellation or deadline errors: evaluation was interrupted.

Invalid input returns no usable decision. An empty policy is not an
allow-everything policy. The evaluator does not mutate its inputs or keep state
between calls and can be shared by concurrent callers.

See [the policy examples](authentication-policy.md) for the YAML representation.
