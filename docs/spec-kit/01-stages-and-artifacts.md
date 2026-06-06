# Spec-Kit Stages & Artifacts (Reference)

This is the file-and-stage reference table. Every spec-kit command is a **prompt file** that the AI coding agent reads when the user types its slash command. The prompts are templates with specific file I/O contracts. This document codifies those contracts.

Path conventions:
- `<root>` = project root (where `.specify/` lives).
- `<F>` = feature directory, e.g. `specs/003-user-auth/` or `specs/20260319-143022-user-auth/`.
- Paths are written as the prompts emit them.

---

## Stage 0 — Project Initialization (out-of-band)

Done by `specify init <project-name> --integration <key>` from the CLI, not by an agent command.

**Inputs:**
- Project name
- Chosen integration (claude, copilot, gemini, codex, cursor-agent, …)
- Optional `--here` (init into current dir) or `--no-git` (skip git init)
- Optional flags from each integration (e.g. Codex `--skills` toggle)

**Outputs (typical layout):**

```
<root>/
├── .specify/
│   ├── memory/
│   │   └── constitution.md          # populated/initialized
│   ├── templates/                   # spec-/plan-/tasks-/checklist-template.md
│   ├── extensions.yml               # optional, controls hooks
│   ├── integration.json             # active integration key + metadata
│   ├── init-options.json            # branch_numbering: sequential | timestamp
│   ├── workflow-catalogs.yml        # optional, custom catalog URLs
│   └── workflows/                   # populated by `workflow add`/`run`
│       ├── workflow-registry.json
│       ├── <id>/workflow.yml
│       ├── runs/<run_id>/{state,inputs}.json + log.jsonl
│       └── .cache/
├── specs/                            # empty; one subdir per feature
├── <integration-folder>/             # e.g. .claude/commands/, .gemini/commands/
│   └── commands/                     # the installed prompt files
└── <context-file>                    # e.g. CLAUDE.md, GEMINI.md, AGENTS.md
                                      #   (contains <!-- SPECKIT START --> block)
```

The CLI also writes a `manifest.json` per integration tracking every file it created, so `specify integration uninstall <key>` can remove only what it installed.

---

## Stage 1 — `/speckit.constitution` (OPTIONAL, run once per project)

**Prompt file:** `templates/commands/constitution.md`

**Purpose:** Establish or amend immutable project principles that govern every later spec/plan/task.

**Inputs (read):**
- `$ARGUMENTS` (user message after the slash command — principle text or amendment intent)
- `.specify/memory/constitution.md` (existing; if missing, copied from `.specify/templates/constitution-template.md`)
- `.specify/templates/{plan,spec,tasks}-template.md` (for consistency propagation)
- All command files under `.specify/templates/commands/*.md`
- `README.md`, `docs/quickstart.md`, agent-specific guidance files

**Behavior:**
1. Replace placeholders like `[PROJECT_NAME]`, `[PRINCIPLE_N_NAME]`.
2. Derive values from user input + repo context.
3. Bump `CONSTITUTION_VERSION` per semver rules:
   - MAJOR: principle removal/redefinition
   - MINOR: new principle added
   - PATCH: clarifications, wording
4. **Propagate consistency** by re-reading every dependent template/command and flagging drift.
5. Prepend a **Sync Impact Report** (HTML comment) at the top of the constitution.

**Outputs (write):**
- `.specify/memory/constitution.md` (overwritten)

**Handoffs declared in frontmatter:**
- `speckit.specify` ("Build Specification")

---

## Stage 2 — `/speckit.specify` (REQUIRED)

**Prompt file:** `templates/commands/specify.md`

**Purpose:** Turn a natural-language feature description into a structured spec.

**Inputs (read):**
- `$ARGUMENTS` (the feature description)
- `.specify/init-options.json` → `branch_numbering` (`sequential` vs `timestamp`)
- `templates/spec-template.md`
- Existing `specs/*/` (to compute next NNN if sequential)
- `.specify/extensions.yml` → `hooks.before_specify`, `hooks.after_specify`

**Behavior:**
1. Run **pre-execution hooks** (`before_specify`). Hooks with `optional: false` execute the embedded command; `optional: true` only advertise.
2. Generate a 2–4 word **short name** (action-noun form).
3. Optionally run the **git extension's `before_specify` hook** to create the branch.
4. Compute `SPECIFY_FEATURE_DIRECTORY`:
   - Honor explicit env override if set
   - Else `specs/NNN-<short-name>` (sequential) or `specs/YYYYMMDD-HHMMSS-<short-name>` (timestamp)
5. `mkdir -p` the dir, copy `spec-template.md` → `<F>/spec.md`.
6. Write `.specify/feature.json` = `{"feature_directory": "<resolved path>"}` so later commands can locate it without relying on branch names.
7. Fill the spec from `$ARGUMENTS`:
   - User stories with priorities (P1, P2, P3 …)
   - Functional requirements (testable)
   - Success criteria (measurable, **technology-agnostic**)
   - Key entities (if data involved)
   - Mark up to **3** `[NEEDS CLARIFICATION: ...]` markers max
8. Generate `<F>/checklists/requirements.md` quality checklist.
9. Run validation (max 3 iterations); ask the user up to 3 clarifying questions if any markers remain.
10. Run **post-execution hooks** (`after_specify`).

**Outputs (write):**
- `<F>/spec.md`
- `<F>/checklists/requirements.md`
- `.specify/feature.json`

**Handoffs declared:** `speckit.plan`, `speckit.clarify`

**Key constraints the prompt enforces on the LLM:**
- "Focus on WHAT users need and WHY. Avoid HOW to implement."
- Success criteria must be measurable and tech-agnostic.
- Max 3 clarification markers.

---

## Stage 3 — `/speckit.clarify` (OPTIONAL, but recommended before plan)

**Prompt file:** `templates/commands/clarify.md`

**Purpose:** Targeted ambiguity reduction. Ask up to **5** sharp questions, then encode answers back into the spec.

**Inputs (read):**
- `<F>/spec.md` (located via `scripts/bash/check-prerequisites.sh --json --paths-only`)
- `.specify/extensions.yml` → `hooks.before_clarify` / `after_clarify`

**Behavior:**
1. Structured **ambiguity scan** across 10 taxonomy categories (Functional Scope, Domain/Data, UX, Non-Functional, Integration, Edge Cases, Constraints, Terminology, Completion Signals, Misc).
2. For each Partial/Missing category → candidate question, ranked by impact.
3. Ask **up to 5** in tabled multiple-choice format.
4. Insert answers into a `## Clarifications` section in `spec.md`.

**Outputs (write):**
- `<F>/spec.md` (mutated — adds `## Clarifications` section)

**Handoffs:** `speckit.plan`

---

## Stage 4 — `/speckit.plan` (REQUIRED before tasks)

**Prompt file:** `templates/commands/plan.md`
**Setup script:** `scripts/bash/setup-plan.sh --json` (returns `FEATURE_SPEC`, `IMPL_PLAN`, `SPECS_DIR`, `BRANCH`)

**Purpose:** Translate the spec into a complete technical plan + supporting design artifacts.

**Inputs (read):**
- `<F>/spec.md`
- `.specify/memory/constitution.md`
- `templates/plan-template.md`
- `<context-file>` (e.g. `CLAUDE.md`) — for the agent-context update
- `.specify/extensions.yml` → `hooks.before_plan` / `after_plan`

**Behavior (phased inside the prompt):**

- **Phase setup:** Fill Technical Context. Mark unknowns `NEEDS CLARIFICATION`.
- **Constitution Check:** Evaluate against constitution gates. Hard error if violated without justification.
- **Phase 0 — Research:**
  - For each unknown → research task
  - For each dependency → best-practices task
  - For each integration → patterns task
  - Consolidate into `research.md`: `Decision / Rationale / Alternatives considered`
- **Phase 1 — Design & Contracts:**
  - Extract entities → `data-model.md`
  - Define interface contracts (REST endpoints, CLI schemas, library APIs) → `contracts/*`
  - Generate `quickstart.md` (key validation scenarios)
  - Update the `<!-- SPECKIT START -->`/`<!-- SPECKIT END -->` block in the agent context file to point at the new plan
- **Re-evaluate Constitution Check** post-design.

**Outputs (write):**
- `<F>/plan.md`
- `<F>/research.md`
- `<F>/data-model.md` (if data-bearing)
- `<F>/contracts/*` (if external interfaces)
- `<F>/quickstart.md`
- `<context-file>` (in-place edit, only the SPECKIT-managed block)

**Handoffs:** `speckit.tasks`, `speckit.checklist`

---

## Stage 5 — `/speckit.tasks` (REQUIRED before implement)

**Prompt file:** `templates/commands/tasks.md`
**Setup script:** `scripts/bash/setup-tasks.sh --json` (returns `FEATURE_DIR`, `TASKS_TEMPLATE`, `AVAILABLE_DOCS`)

**Purpose:** Decompose the plan into an actionable, dependency-ordered task list.

**Inputs (read):**
- `<F>/plan.md` (REQUIRED — tech stack, libraries, structure)
- `<F>/spec.md` (REQUIRED — user stories with P1/P2/P3 priorities)
- `<F>/data-model.md` (optional — entities)
- `<F>/contracts/*` (optional — interface contracts)
- `<F>/research.md` (optional — decisions for setup tasks)
- `<F>/quickstart.md` (optional — test scenarios)
- `templates/tasks-template.md`
- `.specify/extensions.yml` → `hooks.before_tasks` / `after_tasks`

**Behavior:**
1. Group tasks by **user story** (PRIMARY organization). Each story = own phase.
2. Map each contract / entity to the story that needs it.
3. Add **Setup** phase (Phase 1) and **Foundational** phase (Phase 2) for shared infra.
4. Mark independent tasks `[P]` (parallelizable).
5. Annotate user-story-phase tasks with `[US1]`, `[US2]`, ...

**Outputs (write):**
- `<F>/tasks.md` (strict format — see below)

**Required task format (enforced by the prompt):**

```
- [ ] T001 Create project structure per implementation plan
- [ ] T005 [P] Implement authentication middleware in src/middleware/auth.py
- [ ] T012 [P] [US1] Create User model in src/models/user.py
```

Components: checkbox · task ID (T###) · `[P]` (optional) · `[US#]` (only for story phases) · description WITH file path.

**Phase ordering:**
1. Setup
2. Foundational (BLOCKING — must complete before any user story)
3. User Story 1 (P1) 🎯 MVP
4. User Story 2 (P2)
5. ... (more stories)
6. Polish & Cross-Cutting

**Handoffs:** `speckit.analyze`, `speckit.implement`

---

## Stage 6 — `/speckit.checklist` (OPTIONAL, may run multiple times)

**Prompt file:** `templates/commands/checklist.md`

**Purpose:** Generate **requirements-quality checklists** for a domain (security, UX, performance, accessibility, …). These are "unit tests for English" — they test the *spec*, not the implementation.

**Inputs (read):**
- `<F>/spec.md`, optionally `<F>/plan.md`, `<F>/tasks.md`
- Up to 3 clarifying questions to determine: scope / depth / audience

**Outputs (write):**
- `<F>/checklists/<domain>.md` (one per invocation; e.g. `security.md`, `ux.md`)

**Used by:** `/speckit.implement` blocks on incomplete checklists.

---

## Stage 7 — `/speckit.analyze` (OPTIONAL, READ-ONLY)

**Prompt file:** `templates/commands/analyze.md`

**Purpose:** Non-destructive cross-artifact consistency scan after `tasks.md` exists.

**Inputs (read):**
- `<F>/spec.md`, `<F>/plan.md`, `<F>/tasks.md` (REQUIRED)
- `/memory/constitution.md`

**Behavior:**
1. Build internal **requirements inventory** (FR-###, SC-###).
2. Build **task coverage map**: which task implements which requirement?
3. Run detection passes:
   - Duplication
   - Missing coverage (FRs without tasks)
   - Constitution violations (auto-CRITICAL)
   - Ambiguity, terminology drift, etc.
4. Cap output at 50 findings; aggregate overflow.

**Outputs:**
- **Inline report only.** Strictly READ-ONLY — no file mutations.
- Offers an optional remediation plan that the user can manually approve.

---

## Stage 8 — `/speckit.implement` (REQUIRED to ship)

**Prompt file:** `templates/commands/implement.md`
**Setup script:** `scripts/bash/check-prerequisites.sh --json --require-tasks --include-tasks`

**Purpose:** Execute the task list, write code.

**Inputs (read):**
- `<F>/tasks.md` (REQUIRED)
- `<F>/plan.md` (REQUIRED)
- `<F>/data-model.md`, `<F>/contracts/`, `<F>/research.md`, `<F>/quickstart.md` (if present)
- `/memory/constitution.md` (if present)
- `<F>/checklists/*` — **blocks execution** if any are incomplete (asks user yes/no to proceed anyway)

**Behavior:**
1. Read every checklist. Tally completed vs incomplete items. Display status table.
2. If any incomplete → ask user "proceed anyway? (yes/no)". Halt on "no".
3. **Project setup verification:** detect tech stack from plan.md, create/verify `.gitignore`, `.dockerignore`, `.eslintignore`, etc. as appropriate.
4. Parse `tasks.md` for phases, dependencies, `[P]` markers.
5. Execute phase-by-phase:
   - Setup → Foundational → US1 → US2 → ... → Polish
   - Sequential vs parallel per task markers
   - Mark each completed task `[X]` in `tasks.md`
6. Halt on non-parallel failure; continue past `[P]` failures with report.

**Outputs (write):**
- Source files per `tasks.md` file paths
- `<F>/tasks.md` (mutated — checks off completed items)
- Ignore files (`.gitignore`, etc.) as needed

---

## Stage 9 — `/speckit.taskstoissues` (OPTIONAL)

**Prompt file:** `templates/commands/taskstoissues.md`

**Purpose:** Push the task list to GitHub Issues (one issue per task) via the `github-mcp-server` MCP tool.

**Inputs (read):** `<F>/tasks.md`, `git config --get remote.origin.url`.

**Outputs:** GitHub Issues (creates one per task in the matching remote repo). Has a CAUTION guard refusing to create in non-matching repos.

---

## Stage Dependency Graph (file-level)

```
constitution.md ──┐
                  ▼
spec.md ──► clarify? ──► plan.md ──► tasks.md ──► implement
   │                         │            │            │
   │                         ▼            ▼            ▼
   │                 research.md      analyze?    source files
   │                 data-model.md    checklists/  tasks.md (mutated)
   │                 contracts/*
   │                 quickstart.md
   │
   └──► checklists/requirements.md   (always)
```

---

## Files Spec-Kit Will Touch / Care About (cheat sheet)

| Path | Owner | Mutability |
|------|-------|-----------|
| `.specify/integration.json` | CLI | Mutated on `integration set` |
| `.specify/init-options.json` | CLI | Immutable post-init |
| `.specify/feature.json` | `/specify` | Overwritten per feature |
| `.specify/memory/constitution.md` | `/constitution` | Versioned amendments |
| `.specify/templates/*` | CLI | Stable; ref source |
| `.specify/extensions.yml` | User | Declares hooks |
| `.specify/workflows/*` | engine | Append-only logs; JSON state |
| `specs/<F>/spec.md` | `/specify`, `/clarify` | Mutated by clarify |
| `specs/<F>/plan.md` | `/plan` | Stable post-plan |
| `specs/<F>/research.md` | `/plan` | Stable |
| `specs/<F>/data-model.md` | `/plan` | Stable |
| `specs/<F>/contracts/*` | `/plan` | Stable |
| `specs/<F>/quickstart.md` | `/plan` | Stable |
| `specs/<F>/tasks.md` | `/tasks`, `/implement` | Mutated (`[X]` marks) |
| `specs/<F>/checklists/*.md` | `/checklist`, `/specify` | Mutated by user |
| `<context-file>` (CLAUDE.md, etc.) | `/plan` | SPECKIT block mutated |
