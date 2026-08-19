---
name: hermoso-design
description: "Use when authoring a Hermoso feature design. Multi-phase interactive process: clarify, explore, propose, deepen, author, document, present. Adapts to feature complexity (small/standard/complex). Produces a design doc and supports deferred approval."
version: 2.0.0
author: Hermoso
license: MIT
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [hermoso, design, interactive, ddd, artifacts]
    related_skills: [hermoso, hermoso-verification, hermoso-construction]
---

# Hermoso Design — Interactive Multi-Phase Process

## Overview

Transform a feature objective into a validated design package (feature-design
JSON + verification contract) through a structured, interactive process that
adapts to the size and complexity of the feature. The process produces
human-readable design artifacts under `docs/features/[slug]/design/` and
culminates in a design approval that can be immediate or deferred.

## Entry Gate

Refresh state — never infer from memory:

```sh
hermoso status <repository> --json
hermoso context <project-id> <feature-id> <run-id> <repository> --json
hermoso schema feature-design --json
hermoso schema feature-verification-contract --json
```

Verify the run is in the `design` phase. If the status is `pending`, signal the
start of active design work:

```sh
hermoso design begin <project-id> <feature-id> <run-id> <repository> --json
```

This transitions the run to `in_progress` and confirms the design phase is
active. If the status is already `in_progress` or `awaiting_approval`, proceed
from where the process left off.

## Phase 1: Clarify

**Goal:** Understand the objective deeply enough to design well.

Ask the user clarifying questions using the `clarify` tool. The number and depth
of questions scale with complexity (see Phase 2 for complexity assessment):

- **Small:** 1-3 questions about scope and acceptance criteria.
- **Standard:** 3-6 questions covering scope, constraints, stakeholders, and
  edge cases.
- **Complex:** 5-10 questions covering scope, constraints, domain boundaries,
  external systems, data flows, failure modes, and stakeholder priorities.

Questions should be multiple-choice when the options are known, open-ended when
the user needs room to explain. Do not ask questions whose answers are already
in the model spine or the user's initial prompt.

Record the user's answers. These inform the design and become part of the
design doc.

## Phase 2: Explore and Assess Complexity

**Goal:** Understand the codebase context and determine feature complexity.

Query the model spine:

```sh
hermoso model query <project-id> <repository> orientation --json
hermoso model query <project-id> <repository> design --json
```

Assess complexity using these heuristics:

- **Small:** Single file or module, no new interfaces, no cross-cutting
  concerns. Example: adding a field to an existing struct, a bug fix with a
  known location, a config flag.
- **Standard:** Multiple files in one package, new methods on existing types,
  small API changes. Example: a new CLI subcommand, a new endpoint on an
  existing handler, extending a validation rule.
- **Complex:** Cross-package changes, new abstractions, domain modeling,
  external system integration, schema migrations, multi-session work.
  Example: a new lifecycle phase, a new domain concept, a major refactor.

Confirm the complexity assessment with the user — present your reasoning and
ask if they agree. The complexity field in the feature-design JSON must match
this assessment.

## Phase 3: Propose Alternatives

**Goal:** Present design options and let the user choose.

For **small** features: propose a single approach. Briefly note why alternatives
were rejected. No need to formally present alternatives unless the user asks.

For **standard** features: propose 2-3 approaches. For each, give:
- A one-paragraph description
- Key trade-offs (simplicity, risk, effort)
- Which surfaces it touches
- A recommendation with rationale

For **complex** features: propose 2-3 architectural approaches. For each, give:
- A description with a component-level sketch
- Trade-offs (coupling, complexity, testability, extensibility)
- Which surfaces and domain boundaries it affects
- Risks and mitigations
- A recommendation with rationale

Present the alternatives to the user and ask them to choose or combine elements.
Record the decision and rationale.

## Phase 4: Deepen (complex features only)

**Goal:** For complex features, conduct a deeper design session.

For **complex** features only (skip for small/standard), perform one or more of:

- **Domain modeling (DDD):** Identify bounded contexts, aggregates, entities,
  value objects, and domain events. Sketch the domain model. This is
  especially valuable when the feature introduces new domain concepts or
  changes boundaries between existing ones.
- **Event storming:** Walk through the key scenarios as event sequences. Note
  commands, events, and read models. This surfaces hidden dependencies and
  sequencing constraints.
- **Interface sketching:** Draft the key interfaces, type signatures, or API
  contracts. This catches integration issues early.
- **Failure analysis:** Walk through failure modes and error handling
  strategies. What happens when the database is down? When the external API
  returns garbage? When concurrent writes collide?

Record the outputs of this phase as artifacts under
`docs/features/[slug]/design/`:
- `domain-model.md` — DDD artifacts (if applicable)
- `event-storming.md` — event sequences (if applicable)
- `interface-sketch.md` — interface drafts (if applicable)
- `risk-analysis.md` — failure modes and mitigations

This phase may span multiple sessions. If the user steps away, resume from
where you left off when they return.

## Phase 5: Author Design Contracts

**Goal:** Draft the formal feature-design JSON and verification contract.

This is the existing contract-authoring process, now informed by the preceding
phases:

1. Draft the feature-design JSON:
   ```sh
   hermoso schema feature-design --json
   ```
   - Set `complexity` to the Phase 2 assessment
   - Requirements, acceptance criteria, surfaces, and terms reflect the chosen
     alternative (Phase 3) and deepening (Phase 4)
   - Base model reference comes from the current model snapshot

2. Validate the design:
   ```sh
   hermoso validate feature-design <path> <project-id> <feature-id> <run-id> <repository> --json
   ```

3. Draft the verification contract:
   ```sh
   hermoso schema feature-verification-contract --json
   ```

4. Validate the verification contract:
   ```sh
   hermoso validate feature-verification-contract <path> <project-id> <feature-id> <run-id> <repository> --json
   ```

5. Persist the design:
   ```sh
   hermoso design put <project-id> <feature-id> <run-id> <repository> <design-path> --json
   ```

6. Persist the verification contract:
   ```sh
   hermoso verification put <project-id> <feature-id> <run-id> <repository> <verification-path> --json
   ```

## Phase 6: Document

**Goal:** Produce a human-readable design summary at
`docs/features/[slug]/design/design.md`.

Write a markdown file that includes:

1. **Feature summary** — one paragraph: what this feature does and why.
2. **Complexity assessment** — the complexity level and why.
3. **Clarifications** — the Q&A from Phase 1 (summarized).
4. **Alternatives considered** — the options from Phase 3 and the decision.
5. **Deep design** (complex only) — links to domain-model.md, event-storming.md,
   etc. with a brief summary of each.
6. **Design overview** — requirements, acceptance criteria, surfaces, and terms
   from the feature-design JSON, in prose.
7. **Verification approach** — what the BDD scenarios cover and how.
8. **Open questions** — anything unresolved that needs attention during
   construction.

This doc is the primary artifact a reviewer reads. The JSON contracts are the
machine-validated truth; this doc is the human-readable story.

## Phase 7: Present and Approve

**Goal:** Present the complete design package and obtain approval.

Show the user:
1. The design doc path: `docs/features/[slug]/design/design.md`
2. The design package revision and hash (from the last `verification put` response)
3. A brief summary of what was designed

Then ask for approval using the `clarify` tool with two choices:
- **Approve now** — proceed to construction immediately
- **Stand down for review** — pause here; the user will review the design doc
  and resume later

If the user approves now:

```sh
hermoso approve design <project-id> <feature-id> <run-id> <repository> <revision> <package-hash> <actor> --json
```

If the user stands down:
- Do NOT call `approve design`
- Summarize what was produced and where the artifacts live
- Tell the user to resume by asking Hermes to "approve the design for
  [feature-id]" when they are ready
- The run stays at `awaiting_approval` — the user can take their time

## Resuming a Deferred Design

When the user returns to approve a deferred design:

1. Refresh state:
   ```sh
   hermoso status <repository> --json
   hermoso context <project-id> <feature-id> <run-id> <repository> --json
   ```
2. Confirm the run is at `PhaseDesign/StatusAwaitingApproval`
3. Read the design doc: `docs/features/[slug]/design/design.md`
4. Confirm the user wants to approve (they may want changes — if so, cycle back
   to Phase 5 with a new revision)
5. Call `hermoso approve design` with the current revision and package hash

## Interaction Principles

- **Never skip clarification.** Even for small features, at least confirm scope.
- **Present alternatives before deciding.** The user should choose, not just
  ratify.
- **Adapt depth to complexity.** A small fix does not need DDD. A new domain
  concept does.
- **Write everything down.** Clarifications, alternatives, and decisions go into
  `docs/features/[slug]/design/`. Future-you and future-reviewers need them.
- **Defer gracefully.** If the user needs to think, let them. The state machine
  supports `awaiting_approval` indefinitely.
- **Never infer context.** Always pass explicit project-id, feature-id, run-id,
  repository to every `hermoso` command.

## Artifact Directory Structure

After a complete design process, `docs/features/[slug]/design/` contains:

```
design/
  design.md          — human-readable design summary (always present)
  clarifications.md  — Phase 1 Q&A summary (standard and complex)
  alternatives.md    — Phase 3 alternatives and decision (standard and complex)
  domain-model.md    — Phase 4 DDD artifacts (complex only)
  event-storming.md  — Phase 4 event sequences (complex only)
  interface-sketch.md — Phase 4 interface drafts (complex only)
  risk-analysis.md   — Phase 4 failure modes (complex only)
```

## Common Pitfalls

1. Treating clarification as optional — it is not. Even a confident guess should
   be confirmed.
2. Presenting a single approach and calling it the "only option" — there are
   always alternatives; surface them.
3. Over-engineering small features — if the complexity is small, keep the
   process lightweight.
4. Under-engineering complex features — if the complexity is complex, do the
   DDD work. Skipping it leads to rework.
5. Forgetting to write the design doc — the JSON is for the machine; the doc is
   for the human. Both are required.
6. Calling `approve design` without the user's explicit consent — approval is
   the user's decision, not the skill's.
