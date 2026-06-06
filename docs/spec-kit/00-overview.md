# Spec-Kit: Executive Overview

**Repo:** https://github.com/github/spec-kit
**Researched against:** main branch, commit cloned 2026-05-17
**Document set:** see `INDEX.md` in this directory.

---

## 1. What Spec-Kit Is

Spec-Kit is a **methodology + toolkit** from GitHub for **Spec-Driven Development (SDD)**. Its thesis: invert the traditional power relationship between specs and code. Specifications become the source of truth; code is the generated, regenerable output.

It ships as a Python CLI called `specify` that:

1. **Initializes** a project with a standard directory layout (`.specify/`, `specs/`, `memory/`, `templates/`).
2. **Installs command files** (markdown / TOML / skill formats) into the directory of whichever AI coding agent you use — Claude Code, Copilot, Gemini CLI, Codex CLI, Cursor, Windsurf, etc. (40+ integrations).
3. **Drives the SDD lifecycle** through a fixed set of slash commands the AI agent invokes: `/speckit.specify`, `/speckit.plan`, `/speckit.tasks`, `/speckit.implement`, plus optional `clarify`, `analyze`, `checklist`, `constitution`, `taskstoissues`.
4. **Orchestrates multi-step flows** through a workflow engine (YAML DSL with 10 step types, state persistence, gates, resume).
5. **Extends** through hooks (`.specify/extensions.yml`), presets (alternative command bundles), and a catalog system for sharing workflows.

**Spec-Kit does not run the AI agent.** It installs prompt files. The user's coding agent (Claude Code, Copilot, etc.) is what actually invokes the prompts when the user types `/speckit.specify <description>`. The workflow engine, however, *does* dispatch to the agent CLI non-interactively via each integration's `build_exec_args()` adapter.

---

## 2. The Mental Model in One Picture

```
┌──────────────────────────────────────────────────────────────────────┐
│                        SDD LIFECYCLE                                  │
│                                                                       │
│   IDEA                                                                │
│    │                                                                  │
│    ▼                                                                  │
│  [/speckit.constitution]  (optional, once per project)                │
│    │  produces: .specify/memory/constitution.md                       │
│    ▼                                                                  │
│  [/speckit.specify]   <─── feature description (natural language)    │
│    │  produces: specs/NNN-name/spec.md  +  checklists/requirements.md │
│    ▼                                                                  │
│  [/speckit.clarify]   (optional, up to 5 targeted questions)          │
│    │  mutates:  spec.md (adds Clarifications section)                 │
│    ▼                                                                  │
│  [/speckit.plan]   <─── tech stack hints                              │
│    │  produces: plan.md, research.md, data-model.md,                 │
│    │            contracts/*, quickstart.md, updated agent context     │
│    ▼                                                                  │
│  [/speckit.tasks]                                                     │
│    │  produces: tasks.md (ordered, [P]-marked, [Story]-labeled)       │
│    ▼                                                                  │
│  [/speckit.checklist]  (optional, "unit tests for English")           │
│    │  produces: checklists/<domain>.md                                │
│    ▼                                                                  │
│  [/speckit.analyze]   (optional, READ-ONLY consistency scan)          │
│    │  produces: inline report                                         │
│    ▼                                                                  │
│  [/speckit.implement]                                                 │
│    │  mutates:  source files, marks tasks [X]                         │
│    ▼                                                                  │
│  DONE                                                                 │
└──────────────────────────────────────────────────────────────────────┘
```

Each box is an AI-agent invocation. The arrows are explicit file dependencies, not magic — every downstream command reads concrete files written by upstream commands.

---

## 3. Why This Matters For An Orchestration Tool

Spec-Kit already ships a workflow engine, so your orchestrator will sit *over* or *around* it. The key facts you need to internalize:

| Fact | Implication for your tool |
|------|---------------------------|
| Commands are **prompt files**, not Python. | You can read/inspect/rewrite them. They're declarative. |
| Each command **writes to known paths** under `specs/<feature-dir>/`. | File presence = stage completion. Easy to detect state. |
| Each command **reads upstream artifacts** by path. | You can swap, mock, or pre-populate them. |
| Resume state lives in `.specify/workflows/runs/<run_id>/`. | You can drive a run from outside the engine. |
| The DSL has **10 step types** and a sandboxed expression evaluator. | A richer DSL is feasible — see doc 04. |
| Integration dispatch goes through `IntegrationBase.build_exec_args()`. | You can plug in a new agent without forking spec-kit. |
| `.specify/extensions.yml` provides pre/post hooks per command. | You have a sanctioned extension point without touching the engine. |
| Project's default agent lives in `.specify/integration.json`. | One source of truth for "which agent runs this run." |

---

## 4. Document Map (this directory)

| # | File | What it covers |
|---|------|----------------|
| 00 | `00-overview.md` | This file. Big picture. |
| 01 | `01-stages-and-artifacts.md` | Every stage: inputs, behavior, outputs, side effects. The reference table. |
| 02 | `02-agents-and-prompts.md` | What each "agent" (command prompt) actually does, what constraints it enforces on the LLM, what its handoffs are. |
| 03 | `03-execution-model.md` | How the engine runs a workflow. State machine, persistence, resume, integration dispatch. |
| 04 | `04-workflow-dsl.md` | Full DSL reference: schema, step types, expression language, validation rules. Includes design notes for a richer DSL. |
| 05 | `05-extension-points.md` | Hooks, integrations, presets, catalogs — every sanctioned way to extend spec-kit without forking. |

Read them in order if you're new; jump to 04 if you're designing the new DSL directly.

---

## 5. Quick Glossary

| Term | Meaning |
|------|---------|
| **SDD** | Spec-Driven Development. The methodology spec-kit implements. |
| **Integration** | An AI coding agent adapter (Claude Code, Copilot, Gemini, etc.). 40+ exist. |
| **Command** | A slash-command prompt file (e.g. `speckit.specify.md`). Installed into the integration's command directory. |
| **Feature directory** | `specs/NNN-feature-name/` — the workspace for a single feature. All artifacts live here. |
| **Constitution** | `.specify/memory/constitution.md` — immutable project principles. Loaded as context by every planning/analyze pass. |
| **Workflow** | A YAML definition that chains commands + control flow. |
| **Step** | One unit of execution in a workflow (10 types). |
| **Run** | One execution of a workflow. Has a `run_id`, state directory, resumable. |
| **Catalog** | A registry pointing to remote workflow YAMLs you can `add` to your project. |
| **Hook** | A pre/post command extension declared in `.specify/extensions.yml`. |
| **Preset** | An alternative bundle of command files (e.g. `lean` preset has fewer commands). |
