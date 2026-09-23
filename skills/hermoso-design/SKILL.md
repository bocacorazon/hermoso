---
name: hermoso-design
description: "Use when authoring a Hermoso feature design. Classifies the request (bounded/standard/complex), then follows a path-appropriate interactive process with a hard approval gate before any contract authoring. Produces a design doc and supports deferred approval."
version: 3.0.0
author: Hermoso
license: MIT
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [hermoso, design, interactive, ddd, artifacts]
    related_skills: [hermoso, hermoso-verification, hermoso-construction]
---

# Hermoso Design — Interactive Path-Branching Process

## Overview

Transform a feature objective into a validated design package (feature-design
JSON + verification contract) through a structured, interactive process that
adapts to the size and complexity of the feature. The process produces
human-readable design artifacts under `docs/features/[slug]/design/` and
culminates in a design approval that can be immediate or deferred.

Before any design work, the request is classified into one of three paths:
**bounded**, **standard**, or **complex**. Each path has different ceremony and
a different terminal state, but all share one invariant: a hard approval gate
before any contract authoring or construction dispatch.

## The Hard Gate

<HARD-GATE>
Do NOT author any feature-design JSON, call `hermoso design put`, call
`hermoso verification put`, invoke any construction skill, write any
implementation code, or take any implementation action until you have told
your human partner what you intend to design and they have approved it. This
applies to EVERY task on EVERY path below — the ceremony scales with the task;
the approval gate never does. Presenting a design and starting in the same
breath is skipping the gate.
</HARD-GATE>

## The One-Way Ratchet

The classification ratchet is one-way: **hidden complexity discovered
mid-task upgrades the path — stop, say so, and step up.** Nothing downgrades
mid-task. If you classified as bounded and discover cross-package dependencies,
stop and re-classify as standard or complex. If you classified as standard and
discover new domain abstractions, stop and re-classify as complex.

Announce the upgrade explicitly: "I classified this as bounded, but I'm now
seeing cross-package dependencies. I'm upgrading to standard — here's what
that means for the process."

## Red Flags

| Thought | Reality |
|---------|---------|
| "This is too simple to need a design" | Simple means a short design, not no design. A few sentences in chat, then approval. |
| "I'll call it bounded and skip the spec" | Reaching for a label to skip work IS the doubt — take the heavier path. |
| "It's bounded and the design is obvious — I'll start while they read it" | The gate is the approval, not the design's length. Present, then stop until you hear yes. |
| "I understand this kind of app, so it's bounded" | Bounded measures the repo, not your familiarity. A new project has no existing flow — it is at least standard. |
| "It grew, but I'm almost done — no need to re-classify" | Hidden complexity upgrades the path mid-task. Stop and say so. |

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

## Phase 0: Classify and Announce

**Goal:** Classify the request and announce the path so the user can override
it.

Before your first clarifying question, classify the request and say the
classification out loud — "this looks bounded, so I'll present a short design
here rather than a full multi-phase spec" — so your human partner can override
it.

### Classification Heuristics

- **Bounded** — a well-scoped change to code that already exists in this
  repo: a new flag, a small endpoint, a one-file fix, adding a field to an
  existing struct. Understanding the kind of app is not enough — bounded
  means the flow you are changing is already here to read. If there is no
  existing flow to change, the task is at least standard. Ask the
  clarifying questions that matter (1-3 questions), present a short design
  IN CHAT (a few sentences to a few short paragraphs), and STOP. No
  formal alternatives document, no DDD artifacts. The design contract is
  lightweight but still required — the feature-design JSON is the
  machine-validated truth.

- **Standard** — multiple files in one package, new methods on existing
  types, small API changes. A new CLI subcommand, a new endpoint on an
  existing handler, extending a validation rule. Follow the full
  interactive process: clarifying questions (3-6), alternatives (2-3
  options), contract authoring, design doc.

- **Complex** — cross-package changes, new abstractions, domain modeling,
  external system integration, schema migrations, multi-session work. A
  new lifecycle phase, a new domain concept, a major refactor. Follow the
  full interactive process plus deepening (DDD, event storming, interface
  sketching, failure analysis).

When in doubt between two paths, take the heavier one. The ratchet is
one-way: hidden complexity discovered mid-task upgrades the path — stop, say
so, and step up. Nothing downgrades mid-task.

## Constitution Check

After Phase 0 (Classify and Announce) and before any path-specific work
(Phase 1: Clarify, Bounded Step 1, etc.), perform the constitution check.

### Step 1: Resolve the Constitution Path

Do NOT assume `docs/constitution.md`. Resolve the constitution through this
order and use the first file that exists:

1. `.hermoso/project.json` key `constitution_path` (explicit per-project
   override), if set
2. `docs/constitution.md`
3. `.specify/memory/constitution.md` (Spec Kit convention — many repos that
   adopted Spec Kit first keep their constitution here and nowhere else)

Read the resolved file and record its path plus content hash in the design doc,
so the constitution a design was checked against is auditable. **Never author a
second constitution to satisfy the path** — two constitutions in one repo drift
into contradiction, and a design that passes against one may violate the other.
The block below is correct in spirit (a constitution must exist) and wrong in
mechanism (it assumes exactly one path). See bocacorazon/hermoso#10.

### Step 2: Block if No Constitution

If none of the paths in Step 1 exist, BLOCK. Do NOT proceed to
clarification, alternatives, contract authoring, or any other phase. Direct
the user to run the `hermoso-constitution` skill first:

> No constitution found at `docs/constitution.md`,
> `.specify/memory/constitution.md`, or a `constitution_path` override. A
> constitution is required
> for feature work — it declares the project's governing principles that
> feature designs must not violate. Run the `hermoso-constitution` skill to
> create one, then resume this design.

### Step 3: Check the Feature Design Against Principles

If the constitution exists, read each principle. For each principle, assess
whether the proposed feature design (as understood so far from the user's
prompt and the Phase 0 classification) would violate it.

If a principle is violated and no escape-hatch override is declared in the
design's `decisions` array, BLOCK before `hermoso design put`. Report which
principle is violated and why:

> Constitution principle "[principle name]" is violated by this feature
> design: [explanation of the violation]. To proceed, either revise the
> design to comply, or declare an escape-hatch override in the design's
> decisions array with rationale.

### Step 4: Escape Hatch

The user may explicitly override a constitution principle by declaring an
override in the design's `decisions` array. The override entry must:

- Name the principle being overridden in the `decision` field
- Explain why the override is justified in the `rationale` field

Example:
```json
{
  "decision": "Overrides constitution principle 'No new dependencies' — this feature requires a Tree-sitter Go binding that is not currently a dependency",
  "rationale": "The binding is needed for code structure analysis and has no Go-native alternative. The dependency is scoped to the model package only."
}
```

The skill must:
1. Record the override in the design doc (Phase 6: Document)
2. Require explicit user approval of the override using the `clarify` tool
3. Proceed only after the user approves the override
4. Flag the override during the construction phase (the construction skill
   re-checks the constitution and sees the override in the approved design)

Overrides are formal — recorded, approved, visible. They are not silent. An
override without rationale or without explicit user approval is not a valid
escape hatch.

### Step 5: Spine Constraint Check

If `docs/spine/index.md` exists in the target project, read every entry of
kind `constraint` with status `active` (the index lists them; fetch bodies
with `spine.py cite <id>` from the `repo-spine` skill). Assess whether the
proposed feature design would violate any of them, exactly as with
constitution principles in Step 3: a violation with no escape-hatch override
declared in the design's `decisions` array BLOCKS before `hermoso design put`.
The override entry must name the spine entry id (e.g. `rapid-wren-3`) in the
`decision` field and justify it in `rationale`. If no spine exists, this step
is a no-op — the spine is optional, the constitution is not.

### Step 6: Proceed

If no constitution exists (blocked at Step 2), do not proceed. If the
constitution exists and all principles are satisfied or overridden with
approved escape-hatch entries, proceed to the path-specific phases below.

## Path: Bounded

For bounded features, the process is lightweight but the approval gate is
just as hard.

### Bounded Step 1: Explore Project Context

Check files, docs, recent commits. Understand the existing flow you are
changing. Query the model spine for orientation:

```sh
hermoso model query <project-id> <repository> orientation --json
```

### Bounded Step 2: Ask Clarifying Questions

Ask 1-3 questions — the ones that matter. Prefer multiple choice. Do not ask
questions whose answers are already in the model spine or the user's initial
prompt. One question at a time if the topic needs exploration.

### Bounded Step 3: Present Short Design in Chat

Present the design in chat: approach, files touched, testing strategy. A few
sentences to a few short paragraphs. This is the approval gate — STOP and wait
for an explicit yes. Presenting the design and starting in the same breath is
skipping the gate.

### Bounded Step 4: Get Approval

Use the `clarify` tool with two choices:
- **Approve** — proceed to contract authoring
- **Revise** — the user wants changes; cycle back to Step 3

### Bounded Step 5: Author Lightweight Contracts

After approval, author the feature-design JSON and verification contract.
The contracts are still required — the JSON is the machine-validated truth
and the verification contract defines what "done" means.

**Schema mapping:** the skill path is called "bounded" but the
feature-design JSON `complexity` enum only accepts `"small"`, `"standard"`,
or `"complex"`. Set `complexity` to `"small"` for bounded features.

1. Draft the feature-design JSON:
   ```sh
   hermoso schema feature-design --json
   ```
   - Set `complexity` to `"small"` (the schema enum has no "bounded" value)
   - Requirements, acceptance criteria, surfaces, and terms reflect the
     approved design

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

### Bounded Step 6: Document and Present

Write a concise `docs/features/[slug]/design/design.md` (a few paragraphs:
summary, approach, verification strategy). Present the design doc path and
the design package revision/hash. Ask for final approval using `clarify`:
- **Approve now** — proceed to construction immediately
- **Stand down for review** — pause here; the user will review and resume later

If the user approves now:

```sh
hermoso approve design <project-id> <feature-id> <run-id> <repository> <revision> <package-hash> <actor> --json
```

If the user stands down, do NOT call `approve design`. Tell the user to
resume by asking Hermes to "approve the design for [feature-id]" when ready.

## Path: Standard

For standard features, the full interactive process runs with moderate depth.

### Standard Phase 1: Clarify

**Goal:** Understand the objective deeply enough to design well.

Ask the user clarifying questions using the `clarify` tool. 3-6 questions
covering scope, constraints, stakeholders, and edge cases.

Questions should be multiple-choice when the options are known, open-ended when
the user needs room to explain. Do not ask questions whose answers are already
in the model spine or the user's initial prompt.

Record the user's answers. These inform the design and become part of the
design doc.

### Standard Phase 2: Explore and Assess Complexity

**Goal:** Understand the codebase context and confirm the complexity
assessment.

Query the model spine:

```sh
hermoso model query <project-id> <repository> orientation --json
hermoso model query <project-id> <repository> design --json
```

Confirm the complexity assessment with the user — present your reasoning and
ask if they agree. The complexity field in the feature-design JSON must match
this assessment.

### Standard Phase 3: Propose Alternatives

**Goal:** Present design options and let the user choose.

Propose 2-3 approaches. For each, give:
- A one-paragraph description
- Key trade-offs (simplicity, risk, effort)
- Which surfaces it touches
- A recommendation with rationale

Present the alternatives to the user and ask them to choose or combine
elements. Record the decision and rationale.

### Standard Phase 4: Author Design Contracts

**Goal:** Draft the formal feature-design JSON and verification contract.

This is the existing contract-authoring process, now informed by the
preceding phases:

1. Draft the feature-design JSON:
   ```sh
   hermoso schema feature-design --json
   ```
   - Set `complexity` to the Phase 2 assessment
   - Requirements, acceptance criteria, surfaces, and terms reflect the chosen
     alternative (Phase 3)

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

### Standard Phase 5: Document

**Goal:** Produce a human-readable design summary at
`docs/features/[slug]/design/design.md`.

Write a markdown file that includes:

1. **Feature summary** — one paragraph: what this feature does and why.
2. **Complexity assessment** — the complexity level and why.
3. **Clarifications** — the Q&A from Phase 1 (summarized).
4. **Alternatives considered** — the options from Phase 3 and the decision.
5. **Design overview** — requirements, acceptance criteria, surfaces, and
   terms from the feature-design JSON, in prose.
6. **Verification approach** — what the BDD scenarios cover and how.
7. **Open questions** — anything unresolved that needs attention during
   construction.

This doc is the primary artifact a reviewer reads. The JSON contracts are the
machine-validated truth; this doc is the human-readable story.

### Standard Phase 6: Present and Approve

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

## Path: Complex

For complex features, the full interactive process runs with maximum depth,
including a deepening phase before contract authoring.

### Complex Phase 1: Clarify

**Goal:** Understand the objective deeply enough to design well.

Ask the user clarifying questions using the `clarify` tool. 5-10 questions
covering scope, constraints, domain boundaries, external systems, data flows,
failure modes, and stakeholder priorities.

Questions should be multiple-choice when the options are known, open-ended when
the user needs room to explain. Do not ask questions whose answers are already
in the model spine or the user's initial prompt.

Record the user's answers. These inform the design and become part of the
design doc.

### Complex Phase 2: Explore and Assess Complexity

**Goal:** Understand the codebase context and confirm the complexity
assessment.

Query the model spine:

```sh
hermoso model query <project-id> <repository> orientation --json
hermoso model query <project-id> <repository> design --json
```

Confirm the complexity assessment with the user — present your reasoning and
ask if they agree. The complexity field in the feature-design JSON must match
this assessment.

### Complex Phase 3: Propose Alternatives

**Goal:** Present design options and let the user choose.

Propose 2-3 architectural approaches. For each, give:
- A description with a component-level sketch
- Trade-offs (coupling, complexity, testability, extensibility)
- Which surfaces and domain boundaries it affects
- Risks and mitigations
- A recommendation with rationale

Present the alternatives to the user and ask them to choose or combine
elements. Record the decision and rationale.

### Complex Phase 4: Deepen

**Goal:** For complex features, conduct a deeper design session.

Perform one or more of:

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

### Complex Phase 5: Author Design Contracts

**Goal:** Draft the formal feature-design JSON and verification contract.

This is the existing contract-authoring process, now informed by the
preceding phases:

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

### Complex Phase 6: Document

**Goal:** Produce a human-readable design summary at
`docs/features/[slug]/design/design.md`.

Write a markdown file that includes:

1. **Feature summary** — one paragraph: what this feature does and why.
2. **Complexity assessment** — the complexity level and why.
3. **Clarifications** — the Q&A from Phase 1 (summarized).
4. **Alternatives considered** — the options from Phase 3 and the decision.
5. **Deep design** — links to domain-model.md, event-storming.md, etc. with a
   brief summary of each.
6. **Design overview** — requirements, acceptance criteria, surfaces, and
   terms from the feature-design JSON, in prose.
7. **Verification approach** — what the BDD scenarios cover and how.
8. **Open questions** — anything unresolved that needs attention during
   construction.

This doc is the primary artifact a reviewer reads. The JSON contracts are the
machine-validated truth; this doc is the human-readable story.

### Complex Phase 7: Present and Approve

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
   to the path's contract-authoring phase with a new revision)
5. Call `hermoso approve design` with the current revision and package hash

## Resuming an Interrupted Design

If the user returns mid-process (before contracts are persisted):

1. Refresh state as above.
2. Determine which path was in progress (bounded/standard/complex) and which
   phase. If the path was not yet classified, start from Phase 0.
3. Resume from the incomplete phase. Do not restart from the beginning — use
   the design artifacts already written to pick up where you left off.

## Interaction Principles

- **Never edit `.hermoso` files directly.** All state transitions go through
  the Hermoso CLI. Mutating Hermoso state outside the CLI corrupts the run.
- **Never skip clarification.** Even for bounded features, at least confirm
  scope.
- **Present alternatives before deciding.** The user should choose, not just
  ratify.
- **Adapt depth to complexity.** A bounded fix does not need DDD. A new domain
  concept does.
- **Write everything down.** Clarifications, alternatives, and decisions go
  into `docs/features/[slug]/design/`. Future-you and future-reviewers need
  them.
- **Defer gracefully.** If the user needs to think, let them. The state
  machine supports `awaiting_approval` indefinitely.
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

For bounded features, `design.md` is the only artifact (a few paragraphs).

## Reference Files

- `references/contract-authoring-pitfalls.md` — Schema and workflow pitfalls
  from the first end-to-end v3.0.0 design session. Covers schema_version
  format, vocabulary_version value, existing-surface model_node_id
  requirement, unresolved_questions validation, verification contract hash
  binding, stale submodule gitlink cleanup, and approve design hash format.
- `references/superpowers-pattern-analysis.md` — Analysis of the superpowers
  brainstorming skill patterns that informed the v3.0.0 path-branching design.

## Common Pitfalls

1. Treating clarification as optional — it is not. Even a confident guess
   should be confirmed.
2. Presenting a single approach and calling it the "only option" — there are
   always alternatives; surface them.
3. Over-engineering bounded features — if the complexity is bounded, keep the
   process lightweight.
4. Under-engineering complex features — if the complexity is complex, do the
   DDD work. Skipping it leads to rework.
5. Forgetting to write the design doc — the JSON is for the machine; the doc
   is for the human. Both are required.
6. Calling `approve design` without the user's explicit consent — approval is
   the user's decision, not the skill's.
7. Skipping the hard gate — presenting a design and starting contract
   authoring in the same turn is skipping the gate. Present, then stop until
   you hear yes.
8. Ignoring the ratchet — if hidden complexity surfaces mid-task, stop and
   upgrade the path. Do not barrel through with a lightweight process on a
   heavy task.
9. Classifying as bounded when there is no existing flow to change — bounded
   measures the repo, not your familiarity. A new project has no existing
   flow — it is at least standard.
10. Setting `complexity` to `"bounded"` in the feature-design JSON — the
    schema enum only accepts `"small"`, `"standard"`, or `"complex"`. Map
    the "bounded" path to `"small"` in the JSON.
11. Blocking on a missing `docs/constitution.md` when the repo keeps its
    constitution at `.specify/memory/constitution.md` (Spec Kit convention).
    Resolve the path per the Constitution Check Step 1 order before declaring
    one absent, and never author a second constitution to satisfy the path.
