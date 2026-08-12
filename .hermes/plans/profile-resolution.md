# Per-Repo and Per-Run Profile Resolution

## Problem

Profile selection (which model handles each phase) is currently global — one
`profiles/default.yaml` shared across all repos. Users want to configure
per-phase models at the repo level (e.g. gchat-second-brain: construction=qwen,
review=glm-5.2) and optionally override for a specific run.

## Current State

- `profiles/default.yaml` maps phases to Hermes profile names
- `construction prepare <ctx> <profile-path>` requires the profile path as a
  positional arg; the skill passes it explicitly
- `Project` struct has no profile field
- `Run` struct has no profile field
- `ConstructionState` already has `ProfilePath` (set during prepare, used by
  subsequent construction commands)

## Design

### Profile resolution chain (highest priority first):

1. **Run-level override** — `Run.ProfilePath` (set via `hermoso start --profile <path>`)
2. **Repo-level default** — `Project.ProfilePath` (set via `hermoso init --profile <path>`)
3. **Explicit arg** — `construction prepare <ctx> <path>` (backward compatible)
4. **Error** — if none of the above

### Changes

#### Domain (identity.go, lifecycle.go)
- Add `ProfilePath string` (omitempty) to `Project`
- Add `ProfilePath string` (omitempty) to `Run`

#### State (store.go)
- `Initialize` accepts optional `profilePath` — stores in Project
- `StartRun` accepts optional `profilePath` — stores in Run
- New method `ResolveProfile(ctx, execution) (string, error)` — returns
  run-level path if set, else project-level path if set, else empty string

#### Workflow (service.go)
- `Prepare` makes profilePath optional: when empty, calls `ResolveProfile`

#### App (app.go)
- `init` command: parse `--profile <path>` flag, pass to Initialize
- `start` command: parse `--profile <path>` flag, pass to StartRun
- `construction prepare`: make profile-path arg optional, pass "" to service
  when omitted (service resolves from state)

### Backward Compatibility

- Old project.json/run.json without `profile_path` → zero value "", no error
- `construction prepare <ctx> <path>` still works (explicit arg wins)
- `DisallowUnknownFields` in readStateJSON: adding a new field to struct is
  safe — old JSON files simply don't have the key, Go field gets zero value

## Test Plan (TDD)

1. Project with ProfilePath round-trips through JSON
2. Initialize stores profilePath in project.json
3. Run with ProfilePath round-trips through JSON
4. StartRun stores profilePath in run state
5. ResolveProfile returns run-level when set
6. ResolveProfile falls back to project-level when run-level is empty
7. ResolveProfile returns error when neither is set
8. `hermoso init --profile <path>` stores it
9. `hermoso start --profile <path>` stores it
10. `construction prepare` without explicit path uses resolved profile
