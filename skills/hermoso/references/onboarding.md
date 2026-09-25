# Onboarding a Repository to Hermoso

Worked example from onboarding `gchat-second-brain` (2026-08-11), updated with
`DobONoMo` onboarding findings (2026-08-21).

## Prerequisites

- `hermoso` binary on PATH (build from the Hermoso repo: `cd /home/marcos/Projects/hermoso && go build -o /tmp/hermoso github.com/bocacorazon/hermoso/cmd/hermoso`, then symlink to `~/.local/bin/`). Use the full module path — the relative path `./cmd/hermoso` fails when `cd` is unreliable.
- `hermes` CLI available and gateway running
- Hermoso skills present in the Hermoso repo at `skills/`

## Step 1: Initialize the project

```sh
cd /path/to/target-repo
hermoso init .
```

Creates `.hermoso/project.json` (project ID, remote URL, default branch, kanban tenant) and `.hermoso/state.lock`.

## Step 2: Gitignore the control-plane state

Add to `.gitignore`:

```
# Hermoso control plane state (clone-local)
.hermoso/
```

`.hermoso/` is clone-local state — it must never be committed.

## Step 3: Register Hermoso skills with Hermes

The Hermoso skills (`hermoso`, `hermoso-design`, `hermoso-construction`, `hermoso-verification`, `hermoso-verification-author`) need to be visible to `hermes skills list`.

### Option A: External dirs (preferred)

Edit `~/.hermes/config.yaml` directly — do NOT use `hermes config set` for list values (see pitfall below):

```yaml
skills:
  external_dirs:
  - /home/marcos/Projects/hermoso/skills
```

### Option B: Symlinks (workaround when config editing is blocked)

Symlink each skill directory into the builtin skills location:

```sh
for skill in hermoso-design hermoso-construction hermoso-verification hermoso-verification-author; do
  src="/home/marcos/Projects/hermoso/skills/$skill"
  dst="$HOME/.hermes/skills/software-development/$skill"
  [ -d "$src" ] && [ ! -e "$dst" ] && ln -s "$src" "$dst"
done
```

Note: `hermes config set` stores YAML list values as JSON string literals (e.g. `external_dirs: '["a", "b"]'`), which Hermes cannot parse as a list. This is why direct file editing or symlinks are needed.

## Step 4: Verify with doctor

```sh
sh /home/marcos/Projects/hermoso/scripts/hermoso-doctor.sh
```

Checks:
- Profile JSON structural validity
- `hermoso` executable on PATH
- `hermes` executable on PATH
- `skills.external_dirs` references the Hermoso skills path
- Hermes profiles referenced by the Hermoso profile exist
- Hermes Kanban CLI available
- Required skills visible (kanban-orchestrator, kanban-worker, test-driven-development, systematic-debugging, spike)
- Hermes gateway ready

**Expected doctor false positives with symlink setup (Option B):**

1. `error: add /home/marcos/Projects/hermoso/skills to skills.external_dirs` — the doctor checks for an `external_dirs` config entry, but symlinks don't use it. The skills ARE visible via `hermes skills list`. This is expected.

2. `error: required skill is not visible: kanban-orchestrator` and `error: required skill is not visible: kanban-worker` — these skills have `environments: [kanban]` in their frontmatter, so `hermes skills list` excludes them from the general listing. They ARE on disk at `~/.hermes/skills/devops/kanban-*` and are auto-injected into kanban worker sessions at dispatch time by the Hermes kanban system. This is by design.

All other checks should pass. If you see errors beyond these two with the symlink setup, investigate.

## Step 5: Verify project status

```sh
hermoso status .
```

Should show: `project <project-id>: 0 run(s)`

## Step 6 (Optional): Per-Phase Model Assignment

The Hermoso profile at `profiles/default.yaml` maps lifecycle phases to Hermes profile names. To use different models per phase:

1. Create Hermes profiles:
   ```sh
   hermes profile create local-coder --clone --description "Local LLM for code generation"
   hermes profile create code-reviewer --clone --description "Stronger model for review"
   ```

2. Set each profile's model in its config (`~/.hermes/profiles/<name>/config.yaml`).

3. Edit the Hermoso profile (`profiles/default.yaml`) to map phases:
   ```json
   {
     "phases": {
       "construction": {
         "orchestrator_profile": "code-reviewer",
         "worker_profile": "local-coder"
       },
       "verification": {
         "profile": "code-reviewer"
       },
       "integration": {
         "profile": "code-reviewer"
       }
     }
   }
   ```

4. Run doctor again to verify all referenced profiles exist.

The kanban dispatch system automatically routes work cards to workers using the profile specified for each phase.
