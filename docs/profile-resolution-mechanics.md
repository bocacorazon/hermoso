# Profile Resolution Mechanics

This document explains the internal call chain for per-repo and per-run
profile resolution in Hermoso. It traces each CLI command through the app,
state, workflow, and domain layers.

## Architecture overview

```
┌─────────────────────────────────────────────────────────┐
│                     CLI (user)                           │
│  hermoso init --profile   hermoso start --profile        │
│  hermoso construction prepare                            │
└────────────┬────────────────────────┬───────────────────┘
             │                        │
             ▼                        ▼
┌─────────────────────────┐  ┌────────────────────────────┐
│   app/app.go            │  │   app/app.go               │
│                         │  │                            │
│  runInit()              │  │  runStart()                │
│  runStart()             │  │  runConstruction()         │
│                         │  │                            │
│  takeProfileFlag()      │  │  resolve() → store+context │
│  takeJSONFlag()         │  │                            │
└────────┬────────────────┘  └─────────┬──────────────────┘
         │                             │
         ▼                             ▼
┌─────────────────────────────────────────────────────────┐
│                state/store.go                            │
│                                                          │
│  InitializeWithProfile()  → writes project.json         │
│  StartRunWithProfile()    → writes run state             │
│  Load()                   → reads project + all runs     │
│  ResolveProfile()         → run → project → error        │
│  readProjectUnlocked()    → internal helper              │
│  readRunUnlocked()        → internal helper              │
└────────┬────────────────────────────┬───────────────────┘
         │                            │
         ▼                            ▼
┌─────────────────────────┐  ┌────────────────────────────┐
│  domain/identity.go     │  │  workflow/service.go       │
│                         │  │                            │
│  Project.ProfilePath    │  │  Prepare()                 │
│                         │  │    calls ResolveProfile()  │
│  domain/lifecycle.go    │  │    when no path given      │
│                         │  │    then LoadProfile()      │
│  Run.ProfilePath        │  │                            │
└─────────────────────────┘  └────────────────────────────┘
```

## The three layers

1. **App layer** (`internal/app/app.go`) — parses CLI args, extracts flags,
   dispatches to state or workflow. This is the only layer that touches
   `os.Args`.

2. **State layer** (`internal/state/store.go`) — reads and writes
   `.hermoso/project.json` and run state files on disk. All persistence
   goes through here. The `Store` struct is created by `Load()` or
   `Initialize*()`.

3. **Workflow layer** (`internal/workflow/service.go`) — orchestrates
   phase logic. `Prepare()` is where profile resolution happens at
   construction time: if no explicit profile path is passed, it asks
   the store to resolve one.

The **domain layer** (`internal/domain/`) is just data structs —
`Project` and `Run` carry the `ProfilePath` field but have no logic.

## Sequence diagrams

### 1. `hermoso init <repo> --profile <path>`

Stores the profile path at the project level. This becomes the default
for all future runs in this repository.

```
User          app.go              state/store.go         domain
 │               │                      │                   │
 │  init .       │                      │                   │
 │  --profile p  │                      │                   │
 │  --json       │                      │                   │
 ├──────────────►│                      │                   │
 │               │                      │                   │
 │               │  takeProfileFlag()   │                   │
 │               │  → profilePath = "p" │                   │
 │               │                      │                   │
 │               │  InitializeWithProfile(ctx, ".", "p", now)
 │               ├─────────────────────►│                   │
 │               │                      │                   │
 │               │                      │  DiscoverRepository()
 │               │                      │  → find git root   │
 │               │                      │                   │
 │               │                      │  Build Project{    │
 │               │                      │    ProfilePath:"p",│
 │               │                      │    ...}            │
 │               │                      │                   │
 │               │                      │  writeJSON(        │
 │               │                      │    .hermoso/       │
 │               │                      │    project.json)   │
 │               │                      │                   │
 │               │  ◄── Project, created, err ──────────────│
 │               │                      │                   │
 │               │  emit JSON response  │                   │
 │  ◄────────────┤                      │                   │
 │  exit 0       │                      │                   │
```

Result: `.hermoso/project.json` now contains `"profile_path": "p"`.

### 2. `hermoso start <feature> <repo> --profile <path>`

Stores the profile path at the run level. This overrides the project-level
default for this specific run only.

```
User          app.go              state/store.go         domain
 │               │                      │                   │
 │  start f-1 .  │                      │                   │
 │  --profile p  │                      │                   │
 │  --json       │                      │                   │
 ├──────────────►│                      │                   │
 │               │                      │                   │
 │               │  takeProfileFlag()   │                   │
 │               │  → profilePath = "p" │                   │
 │               │                      │                   │
 │               │  Load(ctx, ".")      │                   │
 │               ├─────────────────────►│                   │
 │               │  ◄── store ──────────│                   │
 │               │                      │                   │
 │               │  store.StartRunWithProfile(             │
 │               │    ctx, "f-1", "p", now)                │
 │               ├─────────────────────►│                   │
 │               │                      │                   │
 │               │                      │  readProjectUnlocked()
 │               │                      │  → project.ProfilePath
 │               │                      │    (may be "" or set at init)
 │               │                      │                   │
 │               │                      │  Build Run{       │
 │               │                      │    ProfilePath:"p",│
 │               │                      │    FeatureID:"f-1",│
 │               │                      │    Phase: design} │
 │               │                      │                   │
 │               │                      │  writeJSON(       │
 │               │                      │    .hermoso/      │
 │               │                      │    runs/<id>.json)│
 │               │                      │                   │
 │               │  ◄── Run, err ───────────────────────────│
 │               │                      │                   │
 │               │  emit JSON response  │                   │
 │  ◄────────────┤                      │                   │
 │  exit 0       │                      │                   │
```

Result: `.hermoso/runs/<run-id>.json` now contains `"profile_path": "p"`.

### 3. `hermoso construction prepare` (no explicit profile path)

This is where the resolution chain runs. When no profile path is passed
as a positional argument, the workflow service asks the store to resolve
one by checking the run first, then the project.

```
User        app.go          workflow/service.go    state/store.go
 │             │                    │                     │
 │ construction │                    │                     │
 │ prepare      │                    │                     │
 │ <ctx args>   │                    │                     │
 ├────────────►│                    │                     │
 │             │                    │                     │
 │             │ resolve()          │                     │
 │             │ → store, execution │                     │
 │             │                    │                     │
 │             │ service.Prepare(   │                     │
 │             │   ctx, execution,  │                     │
 │             │   "")  ← empty     │                     │
 │             ├───────────────────►│                     │
 │             │                    │                     │
 │             │                    │ profilePath == ""   │
 │             │                    │ → need to resolve   │
 │             │                    │                     │
 │             │                    │ store.ResolveProfile│
 │             │                    │   (ctx, execution)  │
 │             │                    ├────────────────────►│
 │             │                    │                     │
 │             │                    │                     │  ┌─────────────────────┐
 │             │                    │                     │  │ ResolveProfile()    │
 │             │                    │                     │  │                     │
 │             │                    │                     │  │ 1. readRunUnlocked()│
 │             │                    │                     │  │    run.ProfilePath  │
 │             │                    │                     │  │    == "p"?          │
 │             │                    │                     │  │    YES → return "p" │
 │             │                    │                     │  │    NO  → step 2     │
 │             │                    │                     │  │                     │
 │             │                    │                     │  │ 2. readProjectUnlocked()
 │             │                    │                     │  │    project.ProfilePath
 │             │                    │                     │  │    == "q"?          │
 │             │                    │                     │  │    YES → return "q" │
 │             │                    │                     │  │    NO  → step 3     │
 │             │                    │                     │  │                     │
 │             │                    │                     │  │ 3. return            │
 │             │                    │                     │  │  ErrProfileNotConfigured
 │             │                    │                     │  └─────────────────────┘
 │             │                    │                     │
 │             │                    │ ◄── "p" or "q" ─────│
 │             │                    │     or error         │
 │             │                    │                     │
 │             │                    │ LoadProfile(path)   │
 │             │                    │ → dispatch.Plan     │
 │             │                    │                     │
 │             │ ◄── Run, Plan, changed, err ─────────────│
 │             │                    │                     │
 │             │ emit JSON response │                     │
 │ ◄────────────┤                    │                     │
 │ exit 0       │                    │                     │
```

### 3b. `hermoso construction prepare <explicit-path>`

When a profile path IS passed as a positional argument, resolution is
skipped entirely — the explicit path wins.

```
User        app.go          workflow/service.go    state/store.go
 │             │                    │                     │
 │ construction │                    │                     │
 │ prepare      │                    │                     │
 │ <ctx> myprof │                    │                     │
 ├────────────►│                    │                     │
 │             │                    │                     │
 │             │ resolve()          │                     │
 │             │ → store, execution │                     │
 │             │ → rest = ["myprof"]│                     │
 │             │                    │                     │
 │             │ service.Prepare(   │                     │
 │             │   ctx, execution,  │                     │
 │             │   "myprof")        │                     │
 │             ├───────────────────►│                     │
 │             │                    │                     │
 │             │                    │ profilePath != ""   │
 │             │                    │ → skip ResolveProfile│
 │             │                    │ → use "myprof"      │
 │             │                    │                     │
 │             │                    │ LoadProfile("myprof")│
 │             │                    │ → dispatch.Plan     │
 │             │                    │                     │
 │             │ ◄── Run, Plan ───────────────────────────│
 │             │ emit JSON response │                     │
 │ ◄────────────┤                    │                     │
 │ exit 0       │                    │                     │
```

## Resolution chain summary

```
                    ┌─────────────────────┐
                    │ construction prepare │
                    └──────────┬──────────┘
                               │
                    profilePath arg given?
                   ┌──────yes──┴──no──────┐
                   │                       │
                   ▼                       ▼
            use explicit path     ResolveProfile()
                                        │
                               ┌──────run.ProfilePath?
                               │      set?
                               ┌──yes──┴──no──┐
                               │               │
                               ▼               ▼
                          return run      project.ProfilePath
                          profile path    set?
                                       ┌──yes──┴──no──┐
                                       │               │
                                       ▼               ▼
                                  return project   ErrProfileNotConfigured
                                  profile path
```

## Where data lives on disk

```
<repo>/
  .hermoso/
    project.json          ← Project.ProfilePath stored here
    state.lock            ← file lock (not profile data)
    runs/
      <run-id>.json       ← Run.ProfilePath stored here
```

`project.json` fields (after init --profile):
```json
{
  "schema_version": "hermoso/project/v1",
  "project_id": "project-...",
  "target": { "repository": "...", "remote_url": "..." },
  "kanban_tenant": "",
  "profile_path": "/abs/path/to/profile.json",
  "created_at": "2026-08-11T..."
}
```

`runs/<run-id>.json` fields (after start --profile):
```json
{
  "feature_id": "feature-1",
  "phase": "design",
  "profile_path": "/abs/path/to/override.json",
  ...
}
```

## Function reference

| Function                      | Layer    | Purpose                                    |
|-------------------------------|----------|--------------------------------------------|
| `takeProfileFlag(args)`       | app      | Extracts `--profile <path>` from CLI args  |
| `runInit()`                   | app      | Handles `hermoso init` command             |
| `runStart()`                  | app      | Handles `hermoso start` command            |
| `runConstruction()`           | app      | Handles `hermoso construction` command     |
| `InitializeWithProfile()`     | state    | Creates project.json with ProfilePath      |
| `StartRunWithProfile()`       | state    | Creates run state with ProfilePath         |
| `ResolveProfile()`            | state    | Fallback chain: run → project → error      |
| `readProjectUnlocked()`       | state    | Reads project.json without file lock       |
| `readRunUnlocked()`           | state    | Reads run state without file lock          |
| `Prepare()`                   | workflow | Calls ResolveProfile when path is empty    |
| `LoadProfile()`               | workflow | Loads the JSON profile file into a struct  |
| `Project.ProfilePath`         | domain   | Field on Project struct (persisted)        |
| `Run.ProfilePath`             | domain   | Field on Run struct (persisted)            |
| `ErrProfileNotConfigured`     | state    | Sentinel error when no profile found       |

## Key invariants

1. `--profile` on `init` sets the repo-level default (stored in project.json).
2. `--profile` on `start` sets a run-level override (stored in run state).
3. An explicit path on `construction prepare` always wins — no resolution.
4. When no explicit path is given, resolution checks run first, then project.
5. If neither run nor project has a profile path, the command fails with
   `ErrProfileNotConfigured`.
6. `ProfilePath` uses `omitempty` in JSON — absent when empty, so old
   project.json and run state files without the field still parse correctly.
