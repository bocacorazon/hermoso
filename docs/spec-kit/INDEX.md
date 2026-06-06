# Spec-Kit Research Documentation

Documentation set for the GitHub **Spec-Kit** framework — researched as the foundation for an orchestration tool that will drive spec-kit flows and provide a richer workflow DSL.

**Researched repo:** https://github.com/github/spec-kit
**Cloned to:** `~/Projects/hermoso/research/spec-kit/` (depth=1, main branch)
**Date:** 2026-05-17

---

## Reading Order

| # | Document | Time | Read first if... |
|---|----------|------|------------------|
| **00** | `00-overview.md` | 5 min | You want the big picture. |
| **01** | `01-stages-and-artifacts.md` | 15 min | You want to know what files exist at each stage and who owns them. |
| **02** | `02-agents-and-prompts.md` | 15 min | You want to understand what each prompt actually instructs the LLM to do. |
| **03** | `03-execution-model.md` | 20 min | You want the engine internals — state machine, persistence, dispatch. |
| **04** | `04-workflow-dsl.md` | 25 min | **Most important for your DSL design.** Full schema + gap analysis. |
| **05** | `05-extension-points.md` | 10 min | You want to know how to extend spec-kit without forking. |

Total ~90 minutes for the full set.

---

## Quick Reference

### The Lifecycle
```
constitution (once) → specify → clarify? → plan → tasks → checklist? → analyze? → implement → taskstoissues?
```

### The Built-in Workflow
The official `speckit` workflow chains `specify → plan → tasks → implement` with `gate` reviews between each. See `examples/speckit-workflow.yml` in this directory.

### The DSL At-a-Glance
- 10 step types: `command`, `prompt`, `shell`, `gate`, `if`, `switch`, `while`, `do-while`, `fan-out`, `fan-in`
- Jinja-subset expressions with 4 filters (`default`, `join`, `contains`, `map`)
- Sequential single-threaded engine (no real parallelism today)
- Resumable via `state.json` persistence after every step

### Known Limitations (your competitive surface)
1. No parallelism (fan-out is sequential)
2. No nested resume (paused inside a control-flow step → re-runs the parent)
3. `requires:` not enforced at runtime
4. No artifact declaration (steps can't say "I produce these files")
5. No retry policies, no per-step timeouts
6. No workflow composition (can't `include` one workflow from another)
7. No event-driven triggers (no cron/webhook in engine)

Full list and design ideas in `04-workflow-dsl.md` Part 2.

---

## Source Code Locations (in `research/spec-kit/`)

Key files to read in source order:

```
src/specify_cli/
├── workflows/
│   ├── base.py              # StepBase, StepContext, StepResult (130 lines)
│   ├── engine.py            # WorkflowEngine, RunState, validation (~900 lines)
│   ├── expressions.py       # Sandboxed evaluator (~300 lines)
│   └── steps/<10 step types>/__init__.py
├── integrations/
│   ├── base.py              # IntegrationBase ABC
│   └── <40+ integrations>/__init__.py
templates/commands/
├── specify.md               # The /speckit.specify prompt
├── plan.md
├── tasks.md
├── implement.md
├── constitution.md
├── clarify.md
├── analyze.md
├── checklist.md
└── taskstoissues.md
workflows/
└── speckit/workflow.yml     # The reference workflow
```

---

## What's In `examples/`

- `examples/speckit-workflow.yml` — verbatim copy of the official `Full SDD Cycle` workflow for reference.

---

## Next Steps for Your Tool

Based on this research, the likely shape of your orchestrator is:

1. **Read & validate** spec-kit workflow YAMLs (backwards compat).
2. **Extend the DSL** with `schema_version: "2.0"` that adds:
   - True parallelism (`parallel:` blocks, working `max_concurrency`)
   - Artifact declarations (`produces:` / `depends_on:`)
   - Retry policies + per-step timeouts
   - Workflow composition (`include:` / sub-workflows)
   - Event-driven triggers (cron, webhooks, file watches)
   - Lifecycle hooks at workflow scope
   - Structured outputs (JSON parsing of agent stdout)
3. **Replace** spec-kit's `WorkflowEngine` with one that:
   - Reads its `state.json` format (forward compat).
   - Adds proper concurrency.
   - Supports fine-grained nested resume.
   - Streams subprocess output back to expressions live.
4. **Reuse** the `IntegrationBase` ABC unchanged — don't reinvent the agent dispatch bridge.
5. **Ship as a catalog** so existing spec-kit users can `specify workflow catalog add <yours>`.

See `04-workflow-dsl.md` Part 2 for detailed design notes on each.
