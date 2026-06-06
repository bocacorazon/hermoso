# Spec-Kit Agents & Prompts

Spec-kit doesn't ship "agents" in the autonomous sense — it ships **prompt files** that constrain whatever AI coding agent is installed (Claude Code, Copilot, Gemini, Codex, Cursor, Windsurf, …). Each prompt is a domain-expert persona for one slice of the SDD lifecycle.

This document describes each prompt as an "agent role" — what it expects, what it produces, and what discipline it imposes on the LLM running it.

---

## How Prompts Reach the Agent

1. `specify init --integration <key>` writes the prompts into the integration's command directory. Locations vary by integration:

   | Integration | Command dir | File ext |
   |-------------|-------------|----------|
   | Claude Code | `.claude/commands/` | `.md` |
   | GitHub Copilot | `.github/prompts/` | `.prompt.md` |
   | Gemini CLI | `.gemini/commands/` | `.toml` |
   | Codex CLI | `.agents/skills/<name>/SKILL.md` | dir/skill |
   | Cursor | `.cursor/commands/` | `.md` |
   | Windsurf | `.windsurf/workflows/` | `.md` |
   | Goose | recipes | `.yaml` |
   | (40+ more) | varies | varies |

2. The user types `/speckit.specify <description>` (or `/speckit-specify` for skills agents) in their agent's UI.
3. The agent reads the prompt file and executes its instructions, treating `$ARGUMENTS` (or `{{args}}` for TOML) as the user's input.

The workflow engine bypasses interactive typing — it invokes the agent CLI directly via `integration.build_exec_args(prompt, model=…, output_json=…)`, then runs it as a subprocess and streams output back.

---

## Agent Role: `speckit.specify` — The Spec Author

**Persona implied by the prompt:** a structured-thinking business analyst who refuses to talk about implementation.

**Core constraints baked into the prompt:**

| Constraint | Effect on LLM behavior |
|------------|------------------------|
| "Focus on WHAT users need and WHY. Avoid HOW to implement." | Strips tech stack, frameworks, APIs from the spec. |
| Max 3 `[NEEDS CLARIFICATION]` markers. | Forces prioritization; rest get informed-guess defaults documented as Assumptions. |
| Mandatory checklist auto-generation. | Self-validation: spec must pass `checklists/requirements.md` before claiming done. |
| Success criteria must be **measurable + tech-agnostic**. | Rejects "API response under 200ms"; accepts "users see results instantly". |
| Max 3 validation iterations. | Bounds rework loops. |
| User stories must carry **P1/P2/P3 priorities**. | Downstream `/tasks` uses this for phase ordering. |

**Decisions delegated to user:** only the 3 prioritized clarifications, presented as multiple-choice tables.

**Handoffs declared in frontmatter:** `speckit.plan`, `speckit.clarify`.

---

## Agent Role: `speckit.clarify` — The Ambiguity Hunter

**Persona:** a senior PM who reads specs with adversarial precision.

**Procedure (deterministic, prompt-encoded):**
1. Run scripted prereq check (no AI). Get `FEATURE_SPEC` path.
2. Load spec and run a **taxonomy scan** across 10 categories. Mark each Clear / Partial / Missing.
3. Generate candidate questions, ranked by impact (only Partial/Missing trigger questions; only when clarification would materially change implementation).
4. Ask **at most 5** questions, formatted as compact A–E option tables.
5. After all answers gathered → write the answers into a `## Clarifications` section in `spec.md` (append-only; never rewrites earlier content).

**What it does NOT do:** It does not regenerate user stories or rewrite functional requirements — it only fills gaps.

**Handoffs:** `speckit.plan`.

---

## Agent Role: `speckit.plan` — The Technical Architect

**Persona:** a senior engineer who maps business requirements to architecture decisions and produces every supporting design artifact in one pass.

**Phased execution (prompt-encoded):**

| Phase | Outputs |
|-------|---------|
| **Setup** | Fill Technical Context (mark unknowns NEEDS CLARIFICATION). |
| **Constitution Check** | Evaluate constitution gates. Hard-error if violated without justification documented in "Complexity Tracking". |
| **Phase 0: Research** | Dispatch research tasks per unknown/dependency/integration → consolidate into `research.md` (Decision / Rationale / Alternatives). |
| **Phase 1: Design** | Extract entities → `data-model.md`. Define contracts → `contracts/`. Write `quickstart.md`. Update agent context file's SPECKIT block. |
| **Re-check** | Re-evaluate Constitution Check post-design. |

**The "Constitution Gates" are the disciplinary core:**

```
Phase -1: Pre-Implementation Gates
├── Simplicity Gate (Article VII):    ≤3 projects? No future-proofing?
├── Anti-Abstraction Gate (Article VIII): Using framework directly? Single model rep?
└── Integration-First Gate (Article IX):  Contracts defined? Contract tests written?
```

Failing gates require explicit "Complexity Tracking" entries — the prompt forces the LLM to justify every layer of abstraction in writing.

**Key rules in the prompt:**
- Use absolute paths for filesystem ops; relative paths in documentation refs.
- ERROR (don't skip) on gate failures or unresolved NEEDS CLARIFICATION.

**Handoffs:** `speckit.tasks`, `speckit.checklist`.

---

## Agent Role: `speckit.tasks` — The Decomposer

**Persona:** a tech-lead who breaks epics into shippable, dependency-ordered work items.

**Organization principle (PRIMARY):** **By user story.** Each priority-N story becomes its own phase. Setup and Foundational phases hold cross-story shared work.

**Strict output format (the prompt rejects malformed tasks):**

```
- [ ] T### [P?] [US#?] Description with file/path
```

| Component | Rule |
|-----------|------|
| Checkbox | ALWAYS `- [ ]` |
| Task ID | Sequential `T001…` in execution order |
| `[P]` | Only when parallelizable (different files, no blocked deps) |
| `[US#]` | REQUIRED for user-story-phase tasks; FORBIDDEN for Setup/Foundational/Polish |
| Path | Concrete file path in the description |

**Phase structure (also strict):**
```
Phase 1 — Setup            (project init, shared infra)
Phase 2 — Foundational     (BLOCKING — must complete before any US)
Phase 3+ — User Story N    (in priority order; MVP = US1)
Phase N — Polish           (cross-cutting concerns)
```

**Test tasks are OPTIONAL** — only generated if the spec/user requested TDD or tests explicitly.

**Reporting requirements:**
- Total task count
- Tasks per user story
- Parallel opportunities
- Independent test criteria per story
- Suggested MVP scope (typically US1 only)

**Handoffs:** `speckit.analyze`, `speckit.implement`.

---

## Agent Role: `speckit.implement` — The Builder

**Persona:** a disciplined senior engineer executing a checklist they didn't write, with veto power on checklist gaps.

**Pre-flight (deterministic):**
1. Run checklist gate: read all `<F>/checklists/*.md`. Build a status table.
2. If any checklist has incomplete items → STOP and ask the user "proceed anyway? (yes/no)".

**Project hygiene (also deterministic):**
- Detect tech stack from `plan.md`.
- Create/verify `.gitignore`, `.dockerignore`, `.eslintignore`, `.prettierignore`, `.terraformignore`, `.helmignore` as appropriate, with curated pattern sets per language (extensive table in the prompt).

**Execution loop:**
- Walk phases sequentially.
- Within a phase: respect `[P]` for parallelism, serialize same-file tasks.
- Mark each completed task `[X]` in `tasks.md` immediately on completion.
- Halt on non-parallel failure; for `[P]` failures, continue successful ones and report.

**Completion validation:**
- Verify all required tasks done.
- Confirm tests pass (if generated).
- Validate alignment with spec.

---

## Agent Role: `speckit.analyze` — The Auditor

**Persona:** a meticulous QA lead. **Read-only.** Refuses to mutate files.

**Authority hierarchy hardcoded in the prompt:** Constitution > Spec > Plan > Tasks. Constitution conflicts are auto-CRITICAL — must be fixed by altering the lower artifact, not by reinterpreting the principle.

**Detection passes:**
- Duplication (near-duplicate requirements)
- Missing coverage (FRs without tasks; tasks without FR refs)
- Ambiguity (vague adjectives — "robust", "intuitive" — without quantification)
- Terminology drift
- Constitution violations

**Output cap:** 50 findings + overflow summary, to keep the response token-efficient.

**Side effect:** none. Offers a remediation plan the user must explicitly approve before any other command runs.

---

## Agent Role: `speckit.constitution` — The Lawgiver

**Persona:** a principal engineer + governance officer.

**Procedure:**
1. Load existing constitution; identify `[PLACEHOLDER]` tokens.
2. Collect/derive values from conversation + repo context.
3. Bump version per semver (MAJOR/MINOR/PATCH rules in prompt).
4. **Consistency propagation pass:** re-read `plan-template.md`, `spec-template.md`, `tasks-template.md`, every command file, README, quickstart, and flag drift.
5. Prepend **Sync Impact Report** as HTML comment at top of file.

**Key trait:** This is the only command that systematically *modifies templates* — its consistency pass can touch any other template/command file the user has customized.

**Handoffs:** `speckit.specify`.

---

## Agent Role: `speckit.checklist` — The Requirements Tester

**Persona:** a QA architect who treats specs as code-written-in-English and writes unit tests for them.

**Core distinction the prompt hammers home:**

| ✅ Checklist item (requirements quality) | ❌ Not-a-checklist-item (verification) |
|------------------------------------------|----------------------------------------|
| "Are visual hierarchy requirements defined for all card types?" | "Verify the button clicks correctly" |
| "Is 'prominent display' quantified with sizing/positioning?" | "Test error handling works" |
| "Are hover state requirements consistent across elements?" | "Confirm API returns 200" |

**Procedure:**
1. Up to 3 dynamic clarifying questions (no canned list) to scope: breadth × depth × risk × audience × timing.
2. Generate checklist items grouped by category (e.g. Completeness, Clarity, Consistency, Coverage, Edge Cases).

**Used downstream by:** `/speckit.implement` (gates execution if incomplete).

---

## Agent Role: `speckit.taskstoissues` — The Issue Sync

**Persona:** a project coordinator who fans a task list out to GitHub Issues.

**Key safeguards in the prompt:**
- `ONLY PROCEED TO NEXT STEPS IF THE REMOTE IS A GITHUB URL`
- `UNDER NO CIRCUMSTANCES EVER CREATE ISSUES IN REPOSITORIES THAT DO NOT MATCH THE REMOTE URL`

Uses the `github/github-mcp-server` MCP tool (`issue_write`) for the actual creation.

---

## Cross-Cutting Patterns You'll See in Every Prompt

1. **Hook check before and after.** Every prompt opens with `before_<phase>` hook detection from `.specify/extensions.yml`, ends with `after_<phase>` detection. Mandatory hooks emit `EXECUTE_COMMAND: <cmd>` lines the agent must invoke.
2. **`$ARGUMENTS` is the user payload.** TOML integrations use `{{args}}` instead.
3. **`{SCRIPT}` placeholder** = the shell-detection-aware prerequisite script (`scripts/bash/*.sh` or `scripts/powershell/*.ps1`).
4. **Quoting guidance** (single-quote escape) appears in every prompt that runs scripts.
5. **Frontmatter declares `handoffs:`** — the prompt's "outgoing edges" in the workflow graph.
6. **`__SPECKIT_COMMAND_<NAME>__` placeholders** appear in some prompts. These get substituted at install time with the actual integration-native slash-command name (e.g. `/speckit.plan` for markdown agents, `/speckit-plan` for skills agents).

---

## Implications for an Orchestrator

If you're building over spec-kit, treat each prompt as a black-box LLM call with:

- **Known input contract** (files it reads, plus a string `$ARGUMENTS`).
- **Known output contract** (files it writes; read-only commands declare it).
- **Self-validation behavior** (most commands self-check before claiming done).
- **Handoff declarations** (frontmatter `handoffs:` enumerate the natural next agent).

You can:
- **Pre-populate inputs** to drive a command from data (skip clarify by writing the Clarifications section yourself).
- **Diff outputs** between runs to detect drift.
- **Replay/branch** by copying `<F>` and pointing a new run at it.
- **Inject extra phases** by adding hooks in `extensions.yml` instead of editing prompts.
- **Swap the integration** mid-workflow if your DSL supports per-step integration overrides (spec-kit's already does — `integration: claude` per step).
