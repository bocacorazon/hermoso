---
name: hermoso-design
description: "Use when authoring a spine-grounded Hermoso v2 feature design with stable requirements, vocabulary, and interaction surfaces before the hidden verification contract and work graph."
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

Turn a feature objective into a validated, spine-grounded `feature-design`.
Adapt depth to risk; do not turn small work into a program. The hidden
verification contract is authored next, and the work graph only after the
complete package is approved.
Never edit `.hermoso/**`; persist only through the CLI.

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
hermoso schema feature-verification-contract --json
hermoso model status <project-id> <repository> --json
hermoso model query <project-id> <repository> orientation --json
hermoso model query <project-id> <repository> task <feature objective> --json
```

The checked-in Go types and examples are explanatory only. The command output
is authoritative. Write the artifact outside `.hermoso/**`, then validate:

```sh
hermoso validate feature-design <feature-design-path> <project-id> <feature-id> <run-id> <repository> --json
```

Validation does not persist the design.

If the model is absent or stale, run `hermoso model build <project-id>
<repository> --revision HEAD --json` before drafting. Query interface, testing,
vocabulary, and invariant evidence by node ID. Preserve the exact snapshot
reference in `base_model`; do not copy an unversioned prose view.

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
- Give requirements and acceptance criteria stable semantic IDs; never use array
  positions as downstream references.
- Make acceptance criteria testable and outcome-focused, and link each one to
  its requirement IDs.
- Define business vocabulary IDs, preferred terms, definitions, and aliases.
- Reference observed surfaces by exact model node ID. Declare not-yet-existing
  API, CLI, UI, file, event, or library surfaces as planned overlays.
- Record important tradeoffs as decisions with rationale.
- Include research references only when their revision and SHA-256 hash are
  known.

## Propose a Right-Sized Work Graph

Only after the verification author completes the package and the package is
approved, sketch the likely construction:

- **One item:** valid for a cohesive implementation and its tests.
- **Several independent items:** no parent links; they may run in parallel.
- **Dependency:** add a parent only when the child cannot start without the
  parent's output.
- **Fan-in/integration:** add only for multiple leaves that require a real
  merge, shared validation, or synthesis.

Every work item must cite visible `requirement_ids`,
`acceptance_criterion_ids`, and `surface_ids`, and the graph as a whole must
cover the approved visible design. Every item must also have a profile and an
ordered, non-empty skill list.
Resolve them from the selected profile rather than inventing names. Typical
implementation ordering is:

1. `kanban-worker` (injected by Hermes);
2. `hermoso-construction`;
3. `test-driven-development`;
4. `systematic-debugging` when diagnosing failures.

The proposal is review material, not yet a dispatched graph.

## Approval Presentation

Present:

1. exact feature-design revision and content hash;
2. objective and acceptance criteria;
3. constraints, non-goals, and decisions;
4. complexity assessment;
5. proposed items and true dependencies;
6. any optional spike;
7. important interfaces, data, rollout, or compatibility effects.

Invoke `hermoso-verification-author` after `design put`. Present its coverage,
modalities, exclusions, publication paths, and the exact resulting package
revision/hash. Ask for explicit approval or requested changes. Do not accept
ambiguous assent. Any edited design, verification contract, artifact, or model
snapshot needs a new package revision and approval.

Persist and approve with the complete canonical context:

```sh
hermoso design put <project-id> <feature-id> <run-id> <repository> <feature-design-path> --json
hermoso verification put <project-id> <feature-id> <run-id> <repository> <verification-contract-path> --json
hermoso approve design <project-id> <feature-id> <run-id> <repository> <package-revision> <package-hash> <actor> [comment] --json
```

Stop before construction until all three commands confirm the exact package.

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
- [ ] The knowledge spine was fresh and bounded model queries informed the design.
- [ ] Feature design passed live validation with stable traceability IDs.
- [ ] Complexity and graph size are proportional.
- [ ] Every dependency is necessary.
- [ ] Optional spike resolves a real uncertainty.
- [ ] The hidden verification author completed cross-validation and sealed assets.
- [ ] Exact package revision/hash was presented for one explicit approval.
- [ ] Persistence and exact approval were confirmed by CLI state.
