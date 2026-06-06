# Spec-Kit Extension Points

Spec-kit has four sanctioned extension mechanisms, each appropriate to a different scope. Use these before forking.

---

## 1. Hooks via `.specify/extensions.yml`

The most lightweight extension. Hooks fire before/after each command without modifying the prompt or the engine.

### Location & shape

```yaml
# .specify/extensions.yml
hooks:
  before_specify:
    - extension: my-git
      command: git.feature
      description: "Create a feature branch before specifying"
      prompt: "Branch will be created"
      optional: false                          # false = mandatory, true = advertise only
      condition: "{{ project.uses_git }}"      # optional; opaque to AI, evaluated by HookExecutor
      enabled: true                            # defaults true if omitted
  after_specify:
    - extension: notify
      command: slack.post
      optional: true
  before_plan: [...]
  after_plan: [...]
  before_tasks: [...]
  after_tasks: [...]
  before_clarify: [...]
  after_clarify: [...]
  before_analyze: [...]
  after_analyze: [...]
  before_checklist: [...]
  after_checklist: [...]
  before_implement: [...]
  after_implement: [...]
  before_constitution: [...]
  after_constitution: [...]
  before_taskstoissues: [...]
  after_taskstoissues: [...]
```

### Semantics (from the prompt code)

When a command prompt runs, it:

1. Reads `extensions.yml` (gracefully skipping if missing/malformed).
2. Filters out `enabled: false` hooks.
3. For each remaining hook, the LLM does NOT evaluate `condition:` itself — that's deferred to a `HookExecutor` runtime layer. If `condition` is empty/absent, the hook is executable.
4. Emits a different message based on `optional:`:
   - **Mandatory (`optional: false`)** → emits an `EXECUTE_COMMAND: <cmd>` directive. The AI must invoke that command and wait for its result before continuing.
   - **Optional (`optional: true`)** → only advertises the hook to the user, doesn't auto-run.

### Why this is useful for your orchestrator

- You can inject ANY agent-runnable command at ANY phase boundary without touching prompt files.
- You can wire your own tools (notifications, deploys, validations) into the SDD lifecycle.
- You can disable a stage by registering a mandatory hook that aborts.

### Caveats

- The hook system is half in-prompt (advertising), half in `HookExecutor` (condition eval + dispatch). If you implement your own engine, you need both.
- `condition:` evaluation is left abstract in the source — spec-kit doesn't ship a full evaluator for it (just the prompt instructions to skip).
- Hooks are per-command; there's no global "before_any" or "after_run" hook.

---

## 2. Integrations (AI Agent Adapters)

This is the cleanest extension point. Adding a new AI coding agent is a one-class subclass.

### Anatomy

Every integration is a subclass under `src/specify_cli/integrations/<key>/__init__.py`:

```python
class MyAgentIntegration(MarkdownIntegration):   # or TomlIntegration, YamlIntegration, SkillsIntegration, IntegrationBase
    key = "myagent"

    config = {
        "name":           "MyAgent",
        "folder":         ".myagent/",
        "commands_subdir": "commands",
        "install_url":    "https://...",
        "requires_cli":    True,
    }

    registrar_config = {
        "dir":       ".myagent/commands",
        "format":    "markdown",       # or toml | yaml
        "args":      "$ARGUMENTS",     # or {{args}} for TOML
        "extension": ".md",            # or "/SKILL.md" for skills
    }

    context_file = "MYAGENT.md"        # or None

    # Optionally override for custom behavior:
    def build_exec_args(self, prompt, model=None, output_json=False): ...
    def dispatch_command(self, command, args, model, project_root): ...
```

### Base classes available

| Base class | Use when |
|------------|----------|
| `MarkdownIntegration` | Standard markdown command files (`.md`). Most agents. |
| `TomlIntegration` | TOML-format commands. Gemini CLI uses this. |
| `YamlIntegration` | YAML recipe files. Goose uses this. |
| `SkillsIntegration` | Skill directories (`speckit-<name>/SKILL.md`). Codex CLI, Claude. |
| `IntegrationBase` directly | Anything weirder (custom settings merge, companion files). |

### Registration

Add one import + one `_register()` call in `src/specify_cli/integrations/__init__.py::_register_builtins()`. Both lists are alphabetical.

### Why this is useful for your orchestrator

- If you support agents spec-kit doesn't ship (your own Hermes, internal tools), wrap them as integrations and you get the entire workflow engine for free.
- The `dispatch_command()` abstraction means your DSL doesn't need to know how to talk to any specific agent.

### Caveats

- The `key` MUST match the CLI binary name when `requires_cli: True` (so `shutil.which(key)` works).
- Built-in integrations are registered in code — there's no plugin system for *external* integrations (yet).

---

## 3. Presets (Alternative Command Bundles)

A preset is a curated collection of command files that replace the default spec-kit set.

### Layout

```
presets/<name>/
├── preset.yml                                  # metadata
├── commands/
│   ├── speckit.specify.md
│   ├── speckit.plan.md
│   └── ...
└── templates/                                   # optional override of base templates
    ├── spec-template.md
    └── ...
```

### Built-in presets

- `lean` — 5 commands (no clarify/analyze/checklist).
- `scaffold` — scaffolding example showing how to compose your own commands.
- `self-test` — used by the spec-kit test suite.

### Catalog

Presets have their own catalog (`presets/catalog.json`) just like workflows.

### Why this is useful for your orchestrator

- You can ship an "opinionated" preset that pre-bakes your team's conventions (different prompts, different artifact layouts).
- You can swap individual commands without forking spec-kit (e.g. write your own `speckit.plan` that adds an "architecture decision record" step).

### Caveats

- Presets replace, not extend. If you want to *add* a command, you put it in your preset's `commands/` directory, but you also have to keep the standard commands you didn't intend to override.
- Templates are also overridable — but if you override `spec-template.md`, all your specs use your version.

---

## 4. Workflow Catalogs

Catalogs let you publish workflows for others to install.

### Catalog file format

`workflows/catalog.json`:

```json
{
  "workflows": [
    {
      "id":              "speckit",
      "name":            "Full SDD Cycle",
      "description":     "...",
      "version":         "1.0.0",
      "url":             "https://.../speckit/workflow.yml",
      "author":          "GitHub",
      "tags":            ["sdd", "official"],
      "install_allowed": true
    }
  ]
}
```

### Resolution order (from the engine source)

1. `SPECKIT_WORKFLOW_CATALOG_URL` env var (overrides everything)
2. `.specify/workflow-catalogs.yml` (project)
3. `~/.specify/workflow-catalogs.yml` (user)
4. Built-in defaults:
   - `default` (install allowed)
   - `community` (search only)

Catalogs are cached for 1 hour at `.specify/workflows/.cache/<sha256>.json`.

### Workflow lifecycle

```
specify workflow search <q>      # search across all active catalogs
specify workflow info <id>       # show details
specify workflow add <id>        # download + install
specify workflow run <id>        # execute
specify workflow remove <id>     # uninstall
```

### Why this is useful for your orchestrator

- Your tool can publish its own catalog. Users `specify workflow catalog add <your-url>` and your workflows become discoverable.
- A catalog can host "starter" workflows that wire spec-kit into common patterns (microservice template, ML model lifecycle, etc.).

### Caveats

- Catalogs reference workflow YAMLs by URL. No signing, no verification — `PUBLISHING.md` calls this out as a security concern.
- Workflow versions are advisory.

---

## 5. Slash Command Customization (per-integration)

Each integration can customize *how* slash commands appear. Two main formats:

- **Dot form:** `/speckit.specify` (markdown agents).
- **Hyphen form:** `/speckit-specify` (skills agents like Codex CLI).

Some integrations use entirely different invocation paths (e.g. Goose recipes are invoked by name, not by slash command).

The placeholder `__SPECKIT_COMMAND_SPECIFY__` in prompt files gets replaced at install time with the agent-native form.

---

## 6. Scripts (Bash / PowerShell)

Spec-kit ships parallel bash and PowerShell implementations of:

- `scripts/bash/check-prerequisites.sh` ↔ `scripts/powershell/check-prerequisites.ps1`
- `scripts/bash/common.sh` ↔ `scripts/powershell/common.ps1`
- `scripts/bash/create-new-feature.sh` ↔ `scripts/powershell/create-new-feature.ps1`
- `scripts/bash/setup-plan.sh` ↔ `scripts/powershell/setup-plan.ps1`
- `scripts/bash/setup-tasks.sh` ↔ `scripts/powershell/setup-tasks.ps1`

Each command's prompt frontmatter declares both:

```yaml
scripts:
  sh: scripts/bash/setup-plan.sh --json
  ps: scripts/powershell/setup-plan.ps1 -Json
```

The agent picks the appropriate one based on host detection (or the integration's `format` field).

If you fork or customize, keep both pairs in sync.

---

## 7. Authentication Layer

Spec-kit has its own auth system under `src/specify_cli/authentication/`:

- `github.py` — GitHub OAuth + token helpers
- `azure_devops.py` — Azure DevOps PAT
- `base.py` — abstract auth provider
- `config.py` — auth config storage

This is used by the CLI for catalog access and integration installation (e.g. fetching a private workflow from a private repo). If you build a tool that needs to access user credentials, this is the pattern to follow.

---

## 8. Extension Recipes (Git, etc.)

`extensions/git/` shows the full pattern for "a feature that wires its own commands AND hooks into spec-kit":

```
extensions/git/
├── extension.yml                   # metadata
├── config-template.yml             # what a user's enabled config looks like
├── git-config.yml                  # the actual feature config
├── commands/
│   ├── speckit.git.commit.md
│   ├── speckit.git.feature.md
│   ├── speckit.git.initialize.md
│   ├── speckit.git.remote.md
│   └── speckit.git.validate.md
├── scripts/
│   ├── bash/auto-commit.sh
│   ├── bash/create-new-feature.sh   # <-- the hook target
│   └── powershell/...
└── README.md
```

When installed, the git extension:
1. Adds new commands (`/speckit.git.*`) to the integration's command dir.
2. Registers `hooks.before_specify` pointing at `git.feature` in `.specify/extensions.yml`.
3. Brings its own scripts.

This is a complete worked example for building your own composite extension.

---

## Summary Decision Matrix

When extending spec-kit, pick the lightest mechanism that solves your problem:

| If you need to... | Use this |
|-------------------|----------|
| Run a custom command at a phase boundary | **Hook** (`extensions.yml`) |
| Support a new AI coding agent | **Integration** (subclass + register) |
| Change spec-kit's command prompts | **Preset** (alternative command bundle) |
| Distribute a multi-step orchestration | **Workflow** (+ optional **Catalog** for discovery) |
| Bundle commands + scripts + hooks together | **Extension** (like `extensions/git/`) |
| Add a new control-flow construct to the DSL | Custom **Step Type** registered into `STEP_REGISTRY` |
| Replace the whole engine | Fork — but consider whether a parallel engine that reads spec-kit's state files is cleaner. |
