# Spec-Kit Workflow DSL — Full Reference + Design Notes

This is the complete DSL surface area. The first half is the existing spec — every field, every step type, every expression rule — extracted from the source. The second half is design observations and gap analysis for the richer DSL you intend to build.

---

## Part 1: The Existing DSL (v1.0)

### 1.1 Top-Level Document Structure

```yaml
schema_version: "1.0"                       # REQUIRED. Only "1.0" or "1" accepted.

workflow:
  id:          "speckit"                    # REQUIRED. ^[a-z0-9][a-z0-9-]*[a-z0-9]$
  name:        "Full SDD Cycle"             # REQUIRED.
  version:     "1.0.0"                      # REQUIRED. Semver X.Y.Z.
  author:      "GitHub"                     # optional
  description: "Runs specify → plan → ..."   # optional
  integration: "claude"                      # optional, workflow-level default
  model:       "claude-sonnet-4"             # optional, workflow-level default
  options:     {}                            # optional, workflow-level defaults

requires:                                    # optional, ADVISORY ONLY (not enforced at runtime)
  speckit_version: ">=0.8.5"
  integrations:
    any: ["claude", "copilot", "gemini"]    # non-exhaustive hint

inputs:                                      # optional
  spec:
    type:     string                         # string | number | boolean
    required: true
    prompt:   "Describe what you want to build"
    default:  "..."
    enum:     ["full", "lite"]               # optional membership constraint
  integration:
    type:    string
    default: "auto"                          # special sentinel → reads .specify/integration.json

steps:                                       # REQUIRED, non-empty list
  - id: ...
    type: ...
    ...
```

### 1.2 Field Reference (Cross-Step)

Every step shares these:

| Field | Required | Description |
|-------|----------|-------------|
| `id` | YES | Unique across whole workflow tree. Cannot contain `:` (reserved for nested IDs). |
| `type` | NO | Default `command`. One of: `command`, `prompt`, `shell`, `gate`, `if`, `switch`, `while`, `do-while`, `fan-out`, `fan-in`. |

Step-type-specific fields below.

### 1.3 Step Type Reference

#### `command` (default if `type:` omitted)

```yaml
- id: specify
  command:     speckit.specify              # REQUIRED. The slash-command name.
  integration: "{{ inputs.integration }}"   # optional override
  model:       "claude-sonnet-4"             # optional override
  options:     {temperature: 0.2}            # optional, merged with workflow defaults
  input:
    args: "{{ inputs.spec }}"                # passed as positional arg to the command
```

**Output schema:**
```yaml
output:
  command:      str
  integration:  str
  model:        str
  options:      dict
  input:        dict
  exit_code:    int
  stdout:       str
  stderr:       str
  dispatched:   bool
```

#### `prompt`

```yaml
- id: review-security
  type: prompt
  prompt: "Review {{ inputs.file }} for security vulnerabilities"
  integration: claude
  model: opus
```

Same output schema as `command` (minus `command`, plus `prompt`).

#### `shell`

```yaml
- id: run-tests
  type: shell
  run: "pytest -xvs tests/"
```

**Output schema:** `{exit_code, stdout, stderr}`. Hard timeout: 300s. Uses `shell=True`.

#### `gate`

```yaml
- id: review-spec
  type: gate
  message:   "Review the generated spec."          # REQUIRED (supports expressions)
  options:   [approve, reject]                      # default ["approve", "reject"]
  on_reject: abort                                  # abort | skip | retry
  show_file: "{{ steps.specify.output.file }}"     # optional, displayed inline
```

**Output schema:** `{message, options, on_reject, show_file, choice, aborted?}`.

Validation rule: if `on_reject in {abort, retry}`, `options` MUST contain a "reject" or "abort" choice.

#### `if`

```yaml
- id: branch
  type: if
  condition: "{{ steps.tests.output.exit_code == 0 }}"
  then:
    - id: deploy
      command: speckit.deploy
  else:
    - id: rollback
      type: shell
      run: "git revert HEAD"
```

**Output:** `{condition_result: bool}`.

#### `switch`

```yaml
- id: by-scope
  type: switch
  expression: "{{ inputs.scope }}"
  cases:
    full:
      - id: full-plan
        command: speckit.plan
    backend-only:
      - id: be-plan
        command: speckit.plan
        input: {args: "backend only"}
  default:
    - id: noop
      type: shell
      run: "echo skip"
```

**Output:** `{matched_case: str, expression_value: any}`.

#### `while`

```yaml
- id: poll
  type: while
  condition: "{{ steps.check.output.ready != true }}"
  max_iterations: 5
  steps:
    - id: check
      type: shell
      run: "./scripts/check-ready.sh"
```

`max_iterations` default = 10. Condition evaluated BEFORE each iteration.

#### `do-while`

```yaml
- id: retry-build
  type: do-while
  condition: "{{ steps.build.output.exit_code != 0 }}"
  max_iterations: 3
  steps:
    - id: build
      type: shell
      run: "npm run build"
```

Body runs at least once; condition checked AFTER each iteration.

#### `fan-out`

```yaml
- id: per-feature-spec
  type: fan-out
  items: "{{ inputs.feature_list }}"           # must evaluate to a list
  max_concurrency: 4                            # ACCEPTED BUT NOT ENFORCED — sequential
  step:                                         # template, executed once per item
    id: specify-each
    command: speckit.specify
    input:
      args: "{{ item }}"                        # `item` is bound per iteration
```

**Per-item IDs are auto-generated** as `<parent>:<template_id>:<index>`. Outputs are collected into `output.results` (a list of each item's output).

#### `fan-in`

```yaml
- id: aggregate
  type: fan-in
  wait_for: [specify-1, specify-2, specify-3]   # explicit list of step IDs
  output:
    summary: "{{ fan_in.results | map('exit_code') }}"
    all_ok:  "{{ fan_in.results | map('exit_code') | contains(0) }}"
```

Sets `context.fan_in.results = [<output of each wait_for step>]` while evaluating its own `output:` expressions.

### 1.4 Expression Language

#### Syntax

`{{ expression }}` — Jinja-like delimiter.

#### Namespaces available

| Name | Source | When available |
|------|--------|----------------|
| `inputs` | Resolved workflow inputs | Always |
| `steps` | Accumulated step results | After first step completes |
| `item` | Current fan-out item | Only inside fan-out body |
| `fan_in` | `{"results": [...]}` | Only during fan-in `output:` evaluation |

#### Operators (precedence: high → low)

1. Comparison: `==`, `!=`, `>`, `<`, `>=`, `<=`
2. Membership: `in`, `not in`
3. Logical not: `not <expr>`
4. Logical and: `<expr> and <expr>`
5. Logical or: `<expr> or <expr>`

#### Literals

- Strings: `'foo'` or `"foo"`
- Numbers: `42`, `3.14`
- Booleans: `true`, `false`
- Null: `none`, `null`
- Lists: `[1, 2, 3]`

#### Filters (pipe)

| Filter | Signature | Behavior |
|--------|-----------|----------|
| `default(fallback)` | `value \| default('x')` | Returns fallback if value is `None` or `""` |
| `join(sep)` | `list \| join(', ')` | Joins list to string |
| `contains(sub)` | `value \| contains('x')` | Substring (string) or membership (list) |
| `map(attr)` | `list \| map('a.b')` | Plucks (dot-path) attribute from each dict in list |

#### Resolution modes

- **Single expression** (whole string IS `{{ ... }}`): returns **typed value** (preserves int, bool, list).
- **Mixed template** (contains text outside `{{ }}`): returns **interpolated string** (everything stringified).

This distinction matters for `condition:` fields — write `condition: "{{ x > 0 }}"`, not `condition: "x is {{ x }}"`.

### 1.5 Validation Rules (enforced pre-run)

- Schema version exact match (`"1.0"` or `"1"`).
- Workflow ID regex.
- Workflow version semver.
- Inputs: type whitelist, default coerces, enum validated, required check.
- Step IDs: unique, no `:` characters.
- Step types: must be in `STEP_REGISTRY`.
- Per-step required fields (per step's `validate()`).
- Recursive validation into `then`/`else`/`steps`/`cases.*`/`default`/`step` (fan-out template).
- Fan-out template IDs are validated in a *separate* `seen_ids` set (since engine namespaces them at runtime).

### 1.6 State & Runtime Files

```
.specify/workflows/
├── workflow-registry.json              # installed workflows
├── workflow-catalogs.yml               # project catalog sources
├── .cache/<sha256>.json                # catalog cache, 1h TTL
├── <workflow_id>/workflow.yml          # installed definition
└── runs/<run_id>/
    ├── state.json                      # RunStatus + step_results
    ├── inputs.json                     # resolved inputs
    └── log.jsonl                       # event log
```

### 1.7 Known Limitations (your competitive surface)

| # | Limitation | Source comment |
|---|-----------|----------------|
| 1 | No real parallelism | `fan-out` is sequential; `max_concurrency` accepted but ignored. |
| 2 | No nested resume | If a step inside `if`/`while` pauses, resume re-runs the whole parent. |
| 3 | `requires:` not enforced | `speckit_version` and `integrations.any` are declared but not checked at runtime. |
| 4 | Incomplete stdout capture | `output.stdout`/`stderr` for `command`/`prompt` are marked as "planned enhancement"; exit code is reliable. |
| 5 | No per-step timeouts | Only `shell` has a hardcoded 300s. |
| 6 | No retry policies | Failures fail the run (except gate `retry`). |
| 7 | No artifact declaration | Steps have no way to say "I produce these files." Engine can't detect drift. |
| 8 | No skip/conditional execution by status | Can't say "run this step only if a previous step succeeded" without wrapping every downstream step in `if`. |
| 9 | No subprocess streaming back to expressions | Live stdout doesn't show up in `output.stdout` until completion. |
| 10 | `shell=True` trust | Catalog workflows can `rm -rf /`. No sandboxing. |
| 11 | No dynamic step generation | Can't construct step IDs from data outside fan-out. |
| 12 | No workflow composition | A workflow can't `include` or `call` another workflow. |
| 13 | No event-driven triggers | Workflows are pull-based; no cron or webhook integration in the engine itself. |
| 14 | No structured logging interface | `log.jsonl` is fixed schema; no user-defined log fields. |

---

## Part 2: Design Notes For a Richer DSL

These are observations to inform — not prescribe — your DSL design.

### 2.1 What Spec-Kit Got Right

1. **Inline step definitions in branches.** `then:`/`else:` hold full step definitions, not ID references. This avoids the "ID-to-implementation lookup" problem most YAML DSLs have. Keep this.
2. **Sandboxed expressions.** No `eval()`, no imports. Replicate this — Jinja2's full sandbox is a good upgrade, but match the principle.
3. **Single-vs-mixed expression semantics.** The "single `{{ x }}` returns typed value, mixed returns string" rule is elegant and matches how authors actually write conditionals. Preserve it (or formalize a separate `${expr}` syntax for typed values vs `{{ expr }}` for string interpolation).
4. **State persistence between every step.** This is what makes resume work. Don't shortcut.
5. **Registry-based step types.** Adding a new step type is a one-line registration. Keep this open extension point.
6. **`integration: auto` sentinel resolution.** A great pattern — runtime-resolved defaults from project state. Generalize this: any input could have a `from_project:` resolver.

### 2.2 What's Worth Improving

#### Concurrency & scheduling

The big one. Spec-kit's "sequential, single-threaded" engine wastes the obvious parallelism in fan-out. A richer DSL should:

- Distinguish `parallel: true/false` per fan-out.
- Support max-concurrency that actually works (semaphore over an executor pool).
- Support `parallel:` blocks of arbitrary steps with implicit join.
- Define `output` aggregation semantics (gather-on-success, gather-with-failures, first-success).

Possible shape:

```yaml
- id: parallel-research
  type: parallel
  max_concurrency: 4
  on_failure: continue    # continue | abort | retry
  steps:
    - id: research-a
      command: speckit.research
      input: { topic: "database" }
    - id: research-b
      command: speckit.research
      input: { topic: "auth" }
```

#### Artifact declaration

Steps should declare what they produce so the engine can:

- Detect missing outputs (real error rather than "succeeded" with no file).
- Skip if outputs already exist + inputs unchanged (Make-like cache).
- Build a real DAG without naming every step in `wait_for`.

```yaml
- id: plan
  command: speckit.plan
  produces:
    - "specs/{{ feature_dir }}/plan.md"
    - "specs/{{ feature_dir }}/research.md"
  depends_on:
    - "specs/{{ feature_dir }}/spec.md"
```

#### Retry policy

```yaml
- id: flaky-test
  type: shell
  run: "pytest tests/integration"
  retry:
    max_attempts: 3
    backoff: exponential
    initial_delay: 5s
    retry_on:
      - exit_code: [1, 2]
      - stderr_contains: "Connection refused"
```

#### Per-step timeouts

```yaml
- id: long-task
  command: speckit.implement
  timeout: 45m
  on_timeout: fail        # fail | retry | skip
```

#### Workflow composition

```yaml
- id: sub
  type: include
  workflow: "speckit"                # catalog ID or path
  inputs:
    spec: "{{ inputs.feature_spec }}"
  return:
    feature_dir: "{{ steps.specify.output.feature_dir }}"
```

This is huge for your use case — you can build "macro" workflows that orchestrate multiple spec-kit cycles.

#### Triggers (event-driven runs)

Make workflows callable from:

- Cron (`schedule: "0 9 * * *"`)
- Webhooks (`on: github.pull_request.opened`)
- File watchers (`on: file_change: "specs/**/*.md"`)
- Other workflows (`on: workflow_completed: speckit`)

#### Structured outputs

`command` and `prompt` steps could declare an output schema and parse stdout into it:

```yaml
- id: review
  command: speckit.analyze
  output_format: json    # or yaml | regex | jsonpath
  output_schema:
    findings: list
    critical_count: int
```

Then downstream steps can do `{{ steps.review.output.critical_count > 0 }}` reliably.

#### Skip-by-status helpers

```yaml
- id: deploy
  command: speckit.deploy
  when: "{{ steps.test.status == 'completed' }}"   # no need for outer `if`
  skip_when: "{{ inputs.dry_run }}"
```

#### Hooks at the workflow level (not just command level)

Spec-kit has hooks per-command (via `extensions.yml`). A richer DSL could have lifecycle hooks at the workflow level:

```yaml
lifecycle:
  before_workflow:
    - type: shell
      run: "./scripts/notify-start.sh"
  after_step:                                    # runs after every step
    - type: shell
      run: "echo step done: $STEP_ID"
  on_failure:
    - command: speckit.rollback
  after_workflow:
    - type: shell
      run: "./scripts/notify-end.sh"
```

#### First-class artifacts directory

Rather than every step writing wherever it wants, declare a workflow-scoped workspace:

```yaml
workspace:
  base: ".specify/workflows/runs/{{ run_id }}/workspace/"
  inherit_from: "specs/{{ inputs.feature_dir }}/"
```

This lets you snapshot, diff, and roll back at workflow granularity.

#### Typed inputs beyond scalars

```yaml
inputs:
  feature_list:
    type: array
    item_type: string
    min_items: 1
    max_items: 50
  spec_options:
    type: object
    schema:
      title: string
      priority: { type: string, enum: [P1, P2, P3] }
```

#### Observability primitives

- Per-step trace IDs propagated to subprocesses (env var).
- Optional OpenTelemetry span export.
- `metrics:` block per step (counters, gauges).
- A standardized progress channel that step implementations can write to.

#### DAG-mode vs Sequential-mode

Spec-kit's engine is purely sequential. A richer DSL might let authors opt into DAG mode:

```yaml
mode: dag    # or sequential (default)
steps:
  - id: a
    command: ...
  - id: b
    command: ...
    needs: [a]
  - id: c
    command: ...
    needs: [a]
  - id: d
    command: ...
    needs: [b, c]
```

Engine derives the DAG, runs independent branches in parallel, joins automatically.

### 2.3 Backwards Compatibility Considerations

If you want spec-kit workflow YAMLs to load in your engine unchanged:

- Keep `schema_version: "1.0"` working (treat as a legacy compatibility mode).
- Keep the 10 built-in step types with identical semantics.
- Keep `{{ }}` expression syntax and the 4 filters.
- Keep the `inputs:` schema.

A `schema_version: "2.0"` would unlock all the additions above. The engine can route on `schema_version` to pick the parser/executor.

### 2.4 Migration Surface

If your tool also drives existing spec-kit installations:

- Be prepared to **read** `state.json` files written by spec-kit's engine (forward-compat).
- Be prepared to **resume** spec-kit-created runs without changing their on-disk format.
- Be prepared to **dispatch via existing `IntegrationBase`** classes (don't reinvent the agent-CLI bridge).

### 2.5 Suggested Reading Order in Spec-Kit Source

If you want to implement directly from source, read in this order:

1. `src/specify_cli/workflows/base.py` — 130 lines, defines the interface.
2. `src/specify_cli/workflows/steps/command/__init__.py` — the central step type.
3. `src/specify_cli/workflows/engine.py` — the loop and persistence.
4. `src/specify_cli/workflows/expressions.py` — the expression evaluator.
5. Any other step type to see how control flow works.
6. `src/specify_cli/integrations/base.py` — the agent dispatch ABC.

Total reading: ~2000 lines. The engine itself is ~900 lines of straightforward Python. You can absorb the whole thing in an afternoon.
