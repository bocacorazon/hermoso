---
name: hermoso-design
description: "Use when authoring a right-sized Hermoso feature-design and proposed construction graph. Ground every field in the live schema and stop for explicit exact-revision approval."
version: 1.0.0
author: Hermoso
license: MIT
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [hermoso, design, architecture, contracts, approval]
    related_skills: [hermoso, hermoso-construction]
---

# Hermoso Adaptive Design Author

## Overview

Turn a feature objective into a validated `feature-design` and a proportional
construction proposal. Adapt depth to risk; do not turn small work into a
program. This skill authors artifacts but does not approve, persist, dispatch,
or mutate Hermoso state.

## Inputs

- the complete `hermoso-context/v1` object from
  `hermoso context <project-id> <feature-id> <run-id> <absolute-repository> --json`;
- the absolute workspace (normally the canonical repository for design);
- objective and constraints from the developer;
- selected profile bindings, normally `profiles/default.yaml`.

Never infer identity from cwd or conversation. At entry, before validation, and
before handoff, refresh context, echo project/feature/run/repository, compare
all fields and the absolute workspace, and block on any mismatch.

## Live Contract First

Before drafting:

```sh
hermoso status <repository> --json
hermoso context <project-id> <feature-id> <run-id> <repository> --json
hermoso schema feature-design --json
hermoso schema work-graph --json
```

The checked-in Go types and examples are explanatory only. The command output
is authoritative. Write the artifact outside `.hermoso/**`, then validate:

```sh
hermoso validate feature-design <feature-design-path> <project-id> <feature-id> <run-id> <repository> --json
```

Validation does not persist the design.

## Adaptive Discovery

For **small** work, inspect the relevant code and tests, then produce:

- one clear objective;
- observable acceptance criteria;
- constraints and non-goals only when meaningful;
- the minimum decisions needed to remove ambiguity;
- usually one proposed work item.

For **standard** work, also identify interfaces, data changes, compatibility,
and independent implementation lanes.

For **complex** work, identify architecture boundaries, rollout risks,
cross-component dependencies, and evidence needed at integration. Complexity
does not justify artificial card count.

## Optional Spike

Use the existing `spike` skill only when a high-impact uncertainty needs an
experiment. Reading source or documentation is not a spike. A spike must have:

- a bounded question;
- observable success/failure criteria;
- a runtime budget;
- a throwaway output or an explicit promotion decision.

The spike is optional and should normally precede dependent implementation. Do
not add a spike card as ceremony.

## Feature Design Rules

- Use the schema version returned by the live schema.
- Copy the complete context object exactly; do not reconstruct it field by field.
- Set `producer.skill` to `hermoso-design`.
- Increment revision when changing a previously presented design.
- Keep `unresolved_questions` empty before requesting approval. If questions
  remain, ask or block; do not hide them in prose.
- Make acceptance criteria testable and outcome-focused.
- Record important tradeoffs as decisions with rationale.
- Include research references only when their revision and SHA-256 hash are
  known.

## Propose a Right-Sized Work Graph

After the design validates, sketch the likely construction:

- **One item:** valid for a cohesive implementation and its tests.
- **Several independent items:** no parent links; they may run in parallel.
- **Dependency:** add a parent only when the child cannot start without the
  parent's output.
- **Fan-in/integration:** add only for multiple leaves that require a real
  merge, shared validation, or synthesis.

Every work item must have a profile and an ordered, non-empty skill list.
Resolve them from the selected profile rather than inventing names. Typical
implementation ordering is:

1. `kanban-worker` (injected by Hermes);
2. `hermoso-construction`;
3. `test-driven-development`;
4. `systematic-debugging` when diagnosing failures.

The proposal is review material, not yet a dispatched graph.

## Approval Presentation

Present:

1. exact revision and, once persistence exists, its content hash;
2. objective and acceptance criteria;
3. constraints, non-goals, and decisions;
4. complexity assessment;
5. proposed items and true dependencies;
6. any optional spike;
7. important interfaces, data, rollout, or compatibility effects.

Ask for explicit approval or requested changes. Do not accept ambiguous assent.
Any edited design needs a new revision and new approval.

Persist and approve with the complete canonical context:

```sh
hermoso design put <project-id> <feature-id> <run-id> <repository> <feature-design-path> --json
hermoso approve design <project-id> <feature-id> <run-id> <repository> <revision> <sha256:...> <actor> [comment] --json
```

Stop before construction until both commands confirm the exact revision and
hash. Any later `design put` revision invalidates the prior approval.

## Blocked Design

If a decision, access requirement, or experiment prevents a valid design:

1. explain the blocker precisely;
2. if on Kanban, comment details and call `kanban_block`;
3. optionally author a `phase-result` with phase `design`, status `blocked`, and
   non-empty blockers;
4. validate it with `hermoso validate phase-result <path> <project-id> <feature-id> <run-id> <repository> --json`;
5. do not claim design completion.

## Common Pitfalls

1. Starting with remembered structs instead of `hermoso schema`.
2. Leaving unresolved questions while claiming a valid design.
3. Adding a DAG because orchestration is available.
4. Treating tests as a separate card when the same worker should implement
   with TDD.
5. Treating a spike as production implementation.
6. Writing approval JSON directly into `.hermoso`.

## Verification Checklist

- [ ] Full context came from `hermoso context`, was echoed, and matched status.
- [ ] Absolute workspace matched the canonical repository.
- [ ] Feature design passed live validation.
- [ ] Complexity and graph size are proportional.
- [ ] Every dependency is necessary.
- [ ] Optional spike resolves a real uncertainty.
- [ ] Exact revision was presented for explicit approval.
- [ ] Persistence and exact approval were confirmed by CLI state.
