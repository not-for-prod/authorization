# SDD Prompts

This folder contains reusable prompts for Firestarter's spec-driven development workflow.

Use them in order for non-trivial features:

1. [00-proposal-to-requirements.md](./00-proposal-to-requirements.md)
2. [01-requirements-to-acceptance-criteria.md](./01-requirements-to-acceptance-criteria.md)
3. [02-architecture-constraints.md](./02-architecture-constraints.md)
4. [03-verification.md](./03-verification.md)
5. [04-task-list.md](./04-task-list.md)
6. [execute.md](./execute.md)

Each feature lives in its own folder under `spec/`.

Select that feature directory before running a stage. Every input and output path in the prompts is relative to the selected directory; do not rely on the shell's current working directory or overwrite artifacts from another feature.

The human approval surface is the Mermaid-first event/aggregate model:
sequence diagrams, aggregate boundaries, commands, events, event routes,
decisions, and open questions. After that model is approved, run prompts 00–04
continuously. Their text outputs are machine-facing derivations and do not each
require human approval unless they expose a new observable decision or change
the approved model.

When the user has asked for implementation and verification is ready, execute
the plan without a separate ceremonial plan approval. Pause only for a new
decision, expanded authority/scope, or risky/destructive action.

Example:

```text
spec/001-feature-init/
  proposal.md
  requirements.md
  acceptance_criteria.md
  constraints.md
  verification.md
  plan.md
```
