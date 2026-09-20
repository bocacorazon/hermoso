---
name: hermoso-constitution
description: "Use when creating or amending a project constitution at docs/constitution.md. Interactively guides principle authoring, versioning, and governance metadata for any Hermoso-managed project."
version: 1.0.0
author: Hermoso
license: MIT
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [hermoso, constitution, governance, principles]
    related_skills: [hermoso, hermoso-design, hermoso-construction]
---

# Hermoso Constitution — Project Governance Authoring

## Overview

Create or amend a project's constitution at `docs/constitution.md`. The
constitution is a versioned, human-readable document that declares a project's
governing principles, their rationale, and amendment procedures. Other Hermoso
skills (design, construction) read the constitution as a hard gate — features
that violate principles are blocked unless a formal escape-hatch override is
declared.

This skill works for any Hermoso-managed project, not just the Hermoso repo
itself. The constitution is created in the target project's `docs/` directory.

## Entry Gate

Refresh state — never infer context from cwd, conversation, or branch names:

```sh
hermoso status <repository> --json
hermoso context <project-id> <feature-id> <run-id> <repository> --json
```

Pass explicit project-id, feature-id, run-id, repository to every `hermoso`
command. Never infer identity — the binary validates context against persisted
state and rejects mismatches.

Never edit `.hermoso` files directly. All Hermoso state transitions go through
the Hermoso CLI. Mutating Hermoso state outside the CLI corrupts the run. This
skill writes ONLY to `docs/constitution.md` — never to `.hermoso/`.

## Creating a Constitution

### Step 1: Check for an Existing Constitution

Check for a constitution at these locations, in order:

1. **`docs/constitution.md`** — a Hermoso-managed constitution. If it exists, go to
   Amending a Constitution below.
2. **`.specify/memory/constitution.md`** — a constitution created by speckit (Spec
   Kit). If it exists, create a symlink from `docs/constitution.md` to this file
   and confirm the link. Do NOT create a separate Hermoso constitution — the
   speckit constitution is authoritative:

   ```sh
   ln -sf .specify/memory/constitution.md docs/constitution.md
   git add docs/constitution.md
   ```

   Confirm: "Found an existing speckit constitution at
   `.specify/memory/constitution.md`. Symlinked it from `docs/constitution.md`
   — this is now the project's single source of truth. To amend it, use the
   speckit constitution workflow."

   Then go to Step 5 (Confirm) — do not create a separate constitution.
3. If neither file exists, proceed with creation (Step 2).

### Step 2: Assess the Project

Ask the user about the project's purpose, domain, and key constraints. Query
the model spine for orientation:

```sh
hermoso model query <project-id> <repository> orientation --json
```

Read the project's README, architecture docs, and any existing decision
records to understand its established patterns and values.

### Step 3: Author Principles Interactively

Ask the user clarifying questions about what principles should govern the
project. Use the `clarify` tool with multiple-choice questions when the options
are known, open-ended when the user needs room to explain.

Typical areas to explore:

- **Determinism and reproducibility** — are runs reproducible? Is there hidden
  state?
- **Architecture boundaries** — what are the components and their interfaces?
- **Testing strategy** — what testing discipline is required (TDD, golden
  tests, integration tests)?
- **Skill/Go boundary** — for Hermoso itself: skills do judgment, Go does
  deterministic orchestration.
- **Dependencies and complexity** — YAGNI, minimal deps, static binaries?
- **Security and data hygiene** — no secrets in code/logs, least-privilege.
- **Observability and audit** — structured logs, metrics, audit trails.
- **Idempotency and failure handling** — safe retries, cooperative
  cancellation.

For each principle the user wants, capture:

- **Name** — a short, declarative title (e.g. "Determinism & Reproducibility")
- **Priority** — Non-Negotiable, Mandatory, or Recommended
- **Paragraph** — the rule(s) in declarative language (MUST/SHOULD, not
  "should")
- **Rationale** — why this principle exists and what goes wrong without it

Present the drafted principles to the user for review before writing the file.

### Step 4: Write the Constitution

Write `docs/constitution.md` with the following structure:

```markdown
# [Project Name] Constitution

Guiding principles and quality gates for [one-line project description].

## Core Principles

### I. [Principle Name] ([Priority])

- [Rule statement with MUST/SHOULD]
- [Rule statement]
- [Rationale: why this principle exists]

### II. [Principle Name] ([Priority])

...

## Governance

- **Amendment procedure:** [how principles are added, modified, or removed]
- **Versioning:** semantic versioning — MAJOR for incompatible
  principle removals/redefinitions, MINOR for new principles or material
  expansions, PATCH for clarifications and wording fixes.
- **Compliance review:** feature designs are checked against the constitution
  at design time (hard gate) and construction time (hard gate). Violations
  require a formal escape-hatch override in the design's decisions array.
- **Ratification date:** [ISO date]
- **Last amended:** [ISO date]
- **Constitution version:** [semver]
```

### Step 5: Confirm

Tell the user the constitution has been written and summarize the principles.
The constitution is now active — the `hermoso-design` skill will read it at
its entry gate and block feature designs that violate principles without an
escape-hatch override.

## Amending a Constitution

### Step 1: Read the Existing Constitution

Read `docs/constitution.md`. Parse the current version, principles, and
governance metadata.

### Step 2: Determine What Changed

Ask the user what they want to change — add a principle, modify an existing
one, remove a principle, or update governance metadata.

### Step 3: Bump the Version

Follow semantic versioning:

- **MAJOR:** backward incompatible governance — principle removals or
  redefinitions that change what was previously permitted.
- **MINOR:** new principle added or materially expanded guidance.
- **PATCH:** clarifications, wording fixes, non-semantic refinements.

### Step 4: Write the Amended Constitution

Update `docs/constitution.md` with the changes. Prepend a sync impact report
as an HTML comment at the top:

```html
<!--
SYNC IMPACT REPORT: Amendments to vX.Y.Z ([old date] → [new date])
- Version change: vX.Y.Z → vA.B.C
- Modified principles:
  - [old title → new title if renamed]
- Added sections:
  - [new principle name]
- Removed sections:
  - [removed principle name]
- Follow-up TODOs: [any deferred items]
-->
```

### Step 5: Confirm

Tell the user the constitution has been amended and summarize what changed.
Note that any in-flight feature designs may need re-checking against the
amended constitution.

## Interaction Principles

- **Never edit `.hermoso` files directly.** This skill writes ONLY to
  `docs/constitution.md`. All Hermoso state transitions go through the CLI.
- **Never infer context.** Always pass explicit project-id, feature-id, run-id,
  repository to every `hermoso` command.
- **Principles are declarative.** Use MUST/SHOULD, not "should" or "ought to."
  Vague language is untestable.
- **Write for the project, not for Hermoso.** Each project has its own
  constitution. The Hermoso repo is the first dogfood user, but the skill
  works for any Hermoso-managed project.
- **Present before writing.** Always show the user the drafted principles
  before writing the file. The constitution is governance — the user must
  own it.

## Common Pitfalls

1. Writing a constitution that is too abstract — each principle must be
   testable. "Be good" is not a principle; "Every run is reproducible from
   inputs" is.
2. Forgetting the governance section — without amendment procedure and
   versioning, the constitution becomes stale and unchangeable.
3. Using "should" instead of MUST/SHOULD — "should" is ambiguous. Use the
   RFC 2119 keywords explicitly.
4. Writing principles for Hermoso specifically when the skill should work for
   any project — keep the skill general, let the user author project-specific
   principles.
5. Editing `.hermoso/` state — this skill never touches control-plane state.
   The constitution is a committed markdown file, not Hermoso state.
6. Creating a separate constitution when speckit already has one — check
   `.specify/memory/constitution.md` BEFORE prompting the user to create a new
   constitution. Symlink, don't duplicate. A project with two constitutions
   (one in `.specify/memory/`, one in `docs/`) will inevitably drift.
