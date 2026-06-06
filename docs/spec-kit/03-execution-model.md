# Spec-Kit Execution Model

How a workflow actually runs — engine internals, state machine, dispatch, resume semantics, and integration plumbing. This is what you need to understand before designing your orchestration layer.

Source files (all under `src/specify_cli/workflows/`):
- `__init__.py` — `STEP_REGISTRY`, `_register_builtin_steps()`
- `base.py` — `StepBase`, `StepContext`, `StepResult`, `StepStatus`, `RunStatus`
- `engine.py` — `WorkflowDefinition`, `WorkflowEngine`, `RunState`, `validate_workflow()`, `WorkflowAbortError`
- `expressions.py` — sandboxed expression evaluator
- `steps/<type>/__init__.py` — one file per step type

---

## 1. Run Lifecycle (state machine)

```
              ┌─────────┐
   create ──► │ CREATED │
              └────┬────┘
                   │ start
                   ▼
              ┌─────────┐
              │ RUNNING │ ◄──────────┐
              └────┬────┘            │
       ┌───────────┼───────────┐     │ resume
       │           │           │     │
       ▼           ▼           ▼     │
  ┌────────┐ ┌────────┐  ┌────────┐  │
  │COMPLET-│ │ PAUSED ├──┘        │  │
  │  ED    │ └────────┘  ┌────────┤  │
  └────────┘             │ FAILED ├──┘
                         └────────┘
                         │ABORTED │
                         └────────┘
```

`RunStatus` enum values: `CREATED`, `RUNNING`, `COMPLETED`, `PAUSED`, `FAILED`, `ABORTED`.

`StepStatus` enum values: `PENDING`, `RUNNING`, `COMPLETED`, `FAILED`, `SKIPPED`, `PAUSED`.

`PAUSED` happens on a gate without a TTY (CI, non-interactive). `ABORTED` is reserved for explicit gate rejection with `on_reject: abort`. `FAILED` is everything else.

---

## 2. Top-Level Run Loop

The engine's main loop (paraphrased from `WorkflowEngine._execute_steps()`):

```
for step_config in steps[current_index:]:
    state.current_step_index += 1
    state.current_step_id = step_config["id"]
    state.save()  # persist BEFORE running the step

    step_impl = STEP_REGISTRY[step_config.get("type", "command")]
    result = step_impl.execute(step_config, context)

    # Record result in both context and state
    context.steps[step_id] = {..., "output": result.output}
    state.step_results[step_id] = {..., "output": result.output}
    state.append_log({"event": "step_completed", "step_id": step_id})
    state.save()

    if result.status == PAUSED:
        state.status = PAUSED; state.save(); return
    if result.status == FAILED:
        if config.on_reject == "abort":
            state.status = ABORTED
        else:
            state.status = FAILED
        state.save(); return

    # Control-flow steps return next_steps — recurse
    if result.next_steps:
        self._execute_steps(result.next_steps, context, state, registry,
                             step_offset=-1)
        # Loops: re-evaluate condition, run again up to max_iterations
        # Fan-out: expand template per item with namespaced IDs
```

Key invariants:

- **State is persisted before AND after every step.** A crash mid-step loses at most the in-flight step.
- **`step_offset=-1` for nested steps** means recursive calls don't bump `current_step_index`. Resume re-runs the parent control-flow step and its entire nested body — there's no fine-grained nested resume yet (acknowledged limitation in `ARCHITECTURE.md`).
- **`PAUSED`/`FAILED`/`ABORTED` propagate up** through the recursion immediately.

---

## 3. The StepContext (what steps see)

`StepContext` is the shared blackboard. Every step gets the same instance:

```python
@dataclass
class StepContext:
    inputs:              dict[str, Any]              # resolved workflow inputs
    steps:               dict[str, dict[str, Any]]   # accumulated step results
    item:                Any                          # current fan-out item
    fan_in:              dict[str, Any]               # aggregated fan-in results
    default_integration: str | None                   # workflow-level default
    default_model:       str | None                   # workflow-level default
    default_options:     dict[str, Any]               # workflow-level defaults
    project_root:        str | None
    run_id:              str | None
```

Each step's result lands in `context.steps[step_id]` as:

```python
{
    "integration": "...",
    "model":       "...",
    "options":     {...},
    "input":       {...},
    "output":      {...},   # whatever the step produced
}
```

That's why expressions like `{{ steps.specify.output.exit_code }}` work in downstream steps.

---

## 4. StepResult contract

Every step returns:

```python
@dataclass
class StepResult:
    status:     StepStatus               # COMPLETED / FAILED / PAUSED / SKIPPED
    output:     dict[str, Any]            # arbitrary structured data
    next_steps: list[dict[str, Any]]      # inline step defs for control flow
    error:      str | None
```

Control-flow steps (`if`, `switch`, `while`, `do-while`) populate `next_steps` with the branch/body. The engine recursively executes them. **`fan-out` is special** — see §6.

---

## 5. Input Resolution

When a run starts, inputs are resolved against the workflow's `inputs:` schema by `_resolve_inputs()`:

| Declared type | Coercion rule |
|---------------|---------------|
| `string` | Must be a string. Rejects numbers/lists/dicts (catches YAML `default: 5`). |
| `number` | `float()` then `int()` if whole. **Explicitly rejects bools** (avoids `True → 1` silent coercion). |
| `boolean` | Accepts `True/False`, plus strings `"true"/"1"/"yes"` and `"false"/"0"/"no"`. |
| `enum` | Membership check after type coercion. |

Required inputs without a default and not provided → `ValueError`.

**Special sentinel:** `integration: "auto"` is resolved to the project's actual integration via `.specify/integration.json`. The `enum:` check is bypassed for the literal `"auto"` (since it's a placeholder, not a real key), but the `type:` is still enforced.

---

## 6. Step Type Behaviors (semantic summary)

Pulled directly from each step's `execute()` method:

### `command` step
1. Resolve expressions in `input:` block.
2. Resolve `integration:` (step → workflow default → `context.default_integration`).
3. Resolve `model:`.
4. Merge `options:` (workflow defaults + step overrides).
5. Call `integration.dispatch_command(command, args, model)`.
6. Capture `exit_code`, `stdout`, `stderr`; mark `dispatched: true`.
7. Non-zero exit → `FAILED` with the stderr as the error.

### `prompt` step
Like `command`, but calls `integration.build_exec_args(prompt, model)` directly. Used for free-form prompts not registered as commands.

### `shell` step
- Uses `subprocess.run(shell=True, capture_output=True, text=True, cwd=project_root, timeout=300)`.
- Captures `exit_code`, `stdout`, `stderr`.
- `shell=True` is a deliberate trust decision — workflow authors control commands; catalog-installed workflows need review.

### `gate` step
- **Interactive (TTY):** prints menu, accepts numeric or named choice, blocks on `input()`.
- **Non-interactive (CI):** returns `PAUSED` immediately — caller resumes later.
- `on_reject` modes:
  - `abort` → `FAILED` + run status `ABORTED` (engine handles separately).
  - `skip` → `COMPLETED`, downstream steps decide what to do.
  - `retry` → `PAUSED`; next resume re-executes the gate.

### `if` step
- Evaluates `condition:` (boolean coerced).
- Returns `next_steps = then` or `next_steps = else`.

### `switch` step
- Evaluates `expression:` once, string-coerces the result.
- Matches against `cases:` keys (exact string match).
- Falls through to `default:` list if no case matches.

### `while` step
- Evaluates `condition:` **before** body.
- If truthy → return body via `next_steps`. Engine re-evaluates after iteration.
- `max_iterations` default = 10.

### `do-while` step
- Always returns body once.
- Engine re-evaluates `condition:` between iterations.
- `max_iterations` default = 10.

### `fan-out` step
- Evaluates `items:` to a list.
- Returns metadata only (no `next_steps`).
- Engine handles expansion: for each item, sets `context.item = item`, copies the `step:` template, renames its ID to `<parent>:<template_id>:<index>`, executes it.
- Aggregates per-item outputs into `step_results[<fan-out-id>].output.results`.
- `max_concurrency:` is **accepted but not enforced** — execution is sequential.

### `fan-in` step
- Reads outputs of every step ID listed in `wait_for:`.
- Sets `context.fan_in = {"results": [...]}` while evaluating its own `output:` expressions.
- Restores previous `fan_in` after eval (so nested fan-ins compose).
- Does NOT block on parallel completion (sequential engine anyway).

---

## 7. Expression Evaluator

`expressions.py` implements a **Jinja-subset, sandboxed** evaluator. No imports, no I/O, no arbitrary code.

**Supported:**
- Dot-path: `inputs.spec`, `steps.plan.output.file`
- List indexing: `task_list[0]`
- Comparisons: `==`, `!=`, `>`, `<`, `>=`, `<=`
- Boolean ops: `and`, `or`, `not` (proper precedence: `or` lower than `and`)
- Membership: `in`, `not in`
- Literals: strings (single or double quoted), int, float, true/false, none/null, lists (`[1, 2, 3]`)
- Pipe filters (4):
  - `| default('fallback')` — None/empty → fallback
  - `| join(', ')` — list → string
  - `| contains('sub')` — substring/membership
  - `| map('attr')` — pluck attr (supports dot-path: `map('result.status')`)

**Two evaluation modes:**

- **Single expression** (`"{{ expr }}"` is the whole string) → returns **typed value** (preserves int, bool, list, dict).
- **Mixed template** (`"text {{ expr }} more"`) → returns **interpolated string** (everything stringified).

This matters: writing `condition: "{{ count > 0 }}"` gives you a real boolean; writing `condition: "count is {{ count }}"` gives you a non-empty string (always truthy). Authors get this wrong; you may want to flag it in your DSL's validator.

---

## 8. State Persistence

Per-run state directory: `.specify/workflows/runs/<run_id>/`.

| File | Format | Purpose |
|------|--------|---------|
| `state.json` | JSON | Current run state, step_results dict |
| `inputs.json` | JSON | Resolved input values |
| `log.jsonl` | JSONL | Append-only event log |

`state.json` shape:

```json
{
  "run_id": "8-char-id",
  "workflow_id": "speckit",
  "status": "running",
  "current_step_index": 3,
  "current_step_id": "plan",
  "step_results": {"specify": {...}, "review-spec": {...}, ...},
  "created_at": "2026-05-17T19:38:00Z",
  "updated_at": "2026-05-17T19:41:23Z"
}
```

`log.jsonl` event types observed: `step_completed`, `step_failed`, `workflow_aborted`.

Resume reconstructs `StepContext.steps` from `state.step_results`, sets `current_step_index`, and re-enters the loop.

---

## 9. Integration Dispatch (how commands actually reach the agent CLI)

The engine doesn't know how to talk to Claude or Copilot — each integration knows.

```
CommandStep._try_dispatch(command, integration_key, model, args, context)
    │
    ├─► get_integration(integration_key)        # registry lookup
    │     │
    │     ▼
    │   IntegrationBase impl (one class per agent)
    │     │
    │     └─► dispatch_command(command, args, model, project_root)
    │            │
    │            ├─ resolves slash-command name
    │            │   (.specify → /speckit.specify  OR  /speckit-specify)
    │            ├─ builds exec args via build_exec_args()
    │            └─ subprocess.run(exec_args, cwd=project_root,
    │                              capture_output=True)
    │
    └─► returns {exit_code, stdout, stderr}
```

The exec args differ per agent. Examples:

- **Claude Code:** `claude --print "/speckit.specify <args>"` style.
- **Gemini CLI:** invokes its REPL with the command via TOML.
- **Codex (skills):** invokes the skill by directory name.
- Each integration class implements `build_exec_args()`.

Stdout/stderr are streamed to the terminal AND captured into `output.stdout` / `output.stderr` (full capture is noted as "planned enhancement" in the source — current behavior is just exit code is reliable).

---

## 10. Validation

`validate_workflow(definition)` runs at install/load time and returns a list of error strings:

- Schema version must be `"1.0"` or `"1"`.
- Workflow ID matches `^[a-z0-9][a-z0-9-]*[a-z0-9]$`.
- Version matches `^\d+\.\d+\.\d+$` (semver).
- Inputs:
  - Type ∈ {`string`, `number`, `boolean`}.
  - Defaults are eagerly coerced against their type (catches YAML authoring mistakes).
  - `default: "auto"` on `integration` exempts enum-membership only.
- Steps:
  - Unique IDs across the whole tree.
  - `:` is reserved (engine uses it for namespaced nested IDs).
  - Type ∈ STEP_REGISTRY keys.
  - Per-type validation via `step.validate(config)` (each step has its own required fields).
  - Recursive validation into `then`, `else`, `steps`, `cases.<key>`, `default`, `step` (fan-out template).

If you build a richer DSL, replicate this validation discipline — it catches a *lot* of authoring mistakes pre-run.

---

## 11. Limitations Worth Knowing

These are explicitly called out in code comments and the architecture doc:

1. **No parallelism.** `fan-out` and `max_concurrency` are sequential. The engine has no async scheduler.
2. **No fine-grained nested resume.** Pausing inside a `while`/`if` body restarts the entire control-flow parent and its body on resume.
3. **`requires:` is advisory.** The `speckit_version` and `integrations.any` constraints are declared but not enforced at runtime (planned enhancement).
4. **Full stdout/stderr capture for `command`/`prompt`** is incomplete — exit code is reliable, full text capture is noted as planned.
5. **`shell=True`** in shell steps trusts workflow authors. Catalog-installed workflows need pre-review.
6. **No retry policies** beyond `gate.on_reject: retry`. Failed steps fail the run.
7. **No timeouts per step** except shell's hardcoded 300s.

Each of these is an opportunity for your richer DSL.

---

## 12. Catalog System (workflow discovery)

`WorkflowCatalog.get_active_catalogs()` resolves catalogs in order:

```
1. SPECKIT_WORKFLOW_CATALOG_URL env var  (overrides everything)
2. .specify/workflow-catalogs.yml         (project-level)
3. ~/.specify/workflow-catalogs.yml       (user-level)
4. Built-in defaults:
     - official (install allowed)
     - community (discovery only — search yes, install no)
```

Catalog entries declare `priority` (merge order) and `install_allowed`. Cache is 1-hour TTL in `.specify/workflows/.cache/<sha256>.json`.

`workflow add <id>` downloads the YAML to `.specify/workflows/<id>/workflow.yml` and registers it in `workflow-registry.json`.

---

## TL;DR for an Orchestrator Designer

- Spec-kit's engine is **synchronous, single-threaded, file-driven**.
- Every step boundary persists state — you can intercept between any two steps by reading `state.json`.
- The execution model is essentially: a recursive descent over a YAML tree, with a sandboxed expression evaluator and explicit `PAUSED`/`FAILED` propagation.
- Integration dispatch is **already pluggable** through the `IntegrationBase` ABC — you don't need to fork to add a new agent type.
- The DSL is **closed-set on step types** (10 built-ins) but the registry mechanism (`STEP_REGISTRY`) is extension-friendly — registering a new step type is one `_register_step()` call.
