# Hermoso Process Mechanics

This document explains the entire Hermoso feature development lifecycle: every phase, every CLI command, every state transition, and the internal call chains that connect them. It is intended for operators and skill authors who need to understand what Hermoso does under the hood.

For profile-specific mechanics (per-repo and per-run model configuration), see [profile-resolution-mechanics.md](./profile-resolution-mechanics.md).

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [On-Disk State Layout](#2-on-disk-state-layout)
3. [The State Machine](#3-the-state-machine)
4. [Phase 1: Initialization](#4-phase-1-initialization)
5. [Phase 2: Design](#5-phase-2-design)
6. [Phase 3: Construction](#6-phase-3-construction)
7. [Phase 4: Verification](#7-phase-4-verification)
8. [Phase 5: Release](#8-phase-5-release)
9. [Cross-Cutting: Repository Model](#9-cross-cutting-repository-model)
10. [Cross-Cutting: Git Worktree Management](#10-cross-cutting-git-worktree-management)
11. [Cross-Cutting: Dispatch and Kanban](#11-cross-cutting-dispatch-and-kanban)
12. [Command Reference](#12-command-reference)

---

## 1. Architecture Overview

Hermoso is structured in five layers, each with a single responsibility:

```
┌─────────────────────────────────────────────────────────┐
│                    CLI (app/app.go)                      │
│  Parses args, dispatches to workflow, formats output     │
├─────────────────────────────────────────────────────────┤
│              Workflow (workflow/)                        │
│  service.go       — design + construction orchestration  │
│  verification.go  — verification loop + remediation      │
│  publication.go   — Gherkin publication on pass          │
├─────────────────────────────────────────────────────────┤
│                   State (state/)                         │
│  store.go — project/run persistence, file locking        │
│  Project + Run JSON files under .hermoso/                │
├─────────────────────────────────────────────────────────┤
│              Domain (domain/)                            │
│  identity.go   — Project, ContextRef, Evidence           │
│  lifecycle.go  — Run, phases, statuses, contracts        │
│  Pure types + validation, no I/O                         │
├─────────────────────────────────────────────────────────┤
│           Infrastructure (internal/)                     │
│  repository/  — git worktree + branch manager            │
│  dispatch/    — work graph → Kanban card compiler        │
│  verification/— verification contract validator          │
│  model/       — repository source model (SCIP-based)     │
│  contracts/   — JSON schema generation + validation      │
└─────────────────────────────────────────────────────────┘
```

**Key principle:** The domain layer is pure (no I/O). State does all file I/O with file locking. Workflow orchestrates domain + state + infrastructure. App is a thin command dispatcher.

---

## 2. On-Disk State Layout

When you run `hermoso init` in a repository, Hermoso creates a `.hermoso/` directory:

```
<repo-root>/
  .hermoso/
    project.json          — Project identity (schema_version, project_id, target, kanban_tenant, created_at, profile_path)
    state.lock            — Advisory file lock (flock) for all state operations
    runs/
      <run-id>.json       — One file per run, containing the full Run state
    worktrees/            — Git worktrees for construction work items
      <workspace-id>/     — Each work item gets its own worktree
    models/               — Repository model snapshots (SCIP-based code intelligence)
      <snapshot-id>.json  — Model manifest + graph data
    exclude               — Git exclude entry for .hermoso/ (so it's never committed)
```

**State locking:** All state reads and writes go through `store.withLock(ctx, exclusive, fn)`. This uses `flock` on `state.lock` — shared locks for reads, exclusive locks for writes. This makes Hermoso safe for concurrent invocations.

**Atomic writes:** State files are written atomically: write to a temp file, then `rename` over the target. This prevents partial writes on crash.

---

## 3. The State Machine

A Hermoso run progresses through four phases, each with a set of legal statuses:

```
DESIGN
  pending → in_progress → awaiting_approval
                                    │
                                    ▼
CONSTRUCTION                        │
  in_progress ←─────────────────────┘
    │
    ├──→ blocked
    │
    ▼
VERIFICATION
  awaiting_verification → in_progress
    │
    ├──→ awaiting_judgment ──→ awaiting_remediation ──→ (back to construction)
    │
    ├──→ awaiting_judgment ──→ (resolve surfaces) ──→ awaiting_release
    │
    ▼
RELEASE
  awaiting_release → released
```

### Phase/Status Reference Table

| Phase         | Status                   | Meaning                                         |
|---------------|--------------------------|-------------------------------------------------|
| design        | pending                  | Run created, no design persisted yet            |
| design        | in_progress              | Design contract persisted, not yet approved     |
| design        | awaiting_approval        | Design package complete (feature + verification)|
| construction  | in_progress              | Construction round active                       |
| construction  | blocked                  | A work item is blocked                          |
| verification  | awaiting_verification    | Construction integrated, ready to verify        |
| verification  | in_progress              | Verification run executing                      |
| verification  | awaiting_judgment        | Verification report produced, awaiting skills   |
| verification  | awaiting_remediation     | Judgments say fix needed, new round required    |
| verification  | awaiting_release         | Verification passed, Gherkin published          |
| release       | awaiting_release         | Same as verification's awaiting_release         |
| release       | released                 | Run complete, feature shipped                   |
| (any)         | cancelled                | Run abandoned                                   |
| (any)         | completed                | Terminal success (used for individual rounds)   |

The `statusAllowed(phase, status)` function in `lifecycle.go` enforces these combinations. An invalid phase/status pair fails validation.

---

## 4. Phase 1: Initialization

### Commands

```
hermoso init <repo-path> [--profile <profile-path>]
hermoso start <feature-id> <repo-path> [--profile <profile-path>]
hermoso status <repo-path>
```

### What Happens

**`hermoso init`** creates the project identity:

```
app.runInit()
  → state.InitializeWithProfile(ctx, path, profilePath, now)
      → DiscoverRepository(ctx, start)     — walk up to find .git, read remote URL + default branch
      → Open(repository)                   — create Store
      → store.withLock(exclusive)
          → readProjectUnlocked()          — check if project.json already exists
          → if not exists:
              → newID("project")           — crypto-random hex ID
              → domain.Project{...}        — assemble project identity
              → project.Validate()         — check schema_version, ID format, kanban_tenant
              → writeJSONAtomic(...)       — write project.json atomically
          → registerExclude(excludePath)   — add .hermoso/ to .git/info/exclude
```

**`hermoso start`** creates a run:

```
app.runStart()
  → takeProfileFlag(args)                 — extract --profile if present
  → state.Load(ctx, path)                 — open store, read project + all runs
  → store.StartRunWithProfile(ctx, featureID, profilePath, now)
      → store.withLock(exclusive)
          → readProjectUnlocked()
          → newID("run")                  — crypto-random hex ID
          → domain.Run{
              Phase: PhaseDesign,
              Status: StatusPending,
              Revision: 1,
              Context: ContextRef{...},
              ProfilePath: profilePath,
            }
          → run.Validate()
          → writeJSONAtomic(runs/<run-id>.json, run)
```

**`hermoso status`** reads everything:

```
app.runStatus()
  → state.Load(ctx, path)
      → store.withLock(shared)
          → readProjectUnlocked()
          → readRunsUnlocked(project)     — scan .hermoso/runs/*.json
  → output project + runs
```

### Output

Both `init` and `start` print the project ID and run ID. All commands support `--json` for machine-readable output.

---

## 5. Phase 2: Design

### Commands

```
hermoso design put <project-id> <feature-id> <run-id> <repo> <contract-path>
hermoso verification put <project-id> <feature-id> <run-id> <repo> <contract-path>
hermoso approve design <project-id> <feature-id> <run-id> <repo> <revision> <hash> <actor> [comment]
```

### What Happens

The design phase has three steps: persist the feature design, persist the verification contract, and approve the combined package.

**`design put`** — Persist feature design:

```
app.runDesign()
  → a.resolve(ctx, args)                  — load store, verify project/feature/run match
  → deps.FS.ReadFile(path)                — read feature design JSON from disk
  → service.PutDesign(ctx, execution, data)
      → json.Unmarshal into domain.FeatureDesign
      → feature.Validate()                — check context, requirements, acceptance criteria
      → store.UpdateRun(ctx, execution, func(run *Run))
          → compute feature hash (sha256 of canonical JSON)
          → set run.Design = &DesignState{Feature, FeatureHash, Revision++}
          → set run.Status = StatusInProgress
          → bump run.Revision, run.UpdatedAt
      → writeJSONAtomic(runs/<run-id>.json)
```

**`verification put`** — Persist verification contract (completes the design package):

```
app.runVerificationContract("put")
  → a.resolve(ctx, args)
  → ReadFile(contract-path)
  → json.Unmarshal into domain.FeatureVerificationContract
  → contract.Validate()
  → Read all verification artifacts (e.g. Gherkin .feature files) referenced by the contract
  → service.PutVerificationContract(ctx, execution, data, assets)
      → compute verification_hash (sha256 of contract JSON)
      → compute artifact_root_hash (sha256 of all artifact content hashes)
      → compute package_hash (sha256 of revision + feature_hash + verification_hash + artifact_root_hash + base_model)
      → store.UpdateRun: set run.Design.Verification, run.Design.VerificationHash, etc.
      → set run.Status = StatusAwaitingApproval
```

**`approve design`** — Lock in the design package:

```
app.runApprove()
  → a.resolve(ctx, args)
  → parse revision (uint64), hash, actor, optional comment
  → service.ApproveDesign(ctx, execution, revision, hash, actor, comment)
      → store.UpdateRun:
          → verify revision matches run.Design.Revision
          → verify hash matches run.Design.PackageHash
          → set run.Design.Approval = &Approval{...}
          → set run.Phase = PhaseConstruction
          → set run.Status = StatusInProgress
```

### Key Invariants

- The design package hash is deterministic: `DesignPackageHash(revision, featureHash, verificationHash, artifactRootHash, baseModel)`.
- Approval locks the exact revision + hash. Any mismatch is rejected.
- Phase transition: `design/awaiting_approval` → `construction/in_progress` happens on approval.

---

## 6. Phase 3: Construction

### Commands

```
hermoso graph put <full-context> <graph-path>
hermoso construction prepare <full-context> [profile-path]
hermoso construction ready <full-context>
hermoso work start <full-context> <work-item-id>
hermoso work complete <full-context> <work-item-id> <evidence-id> <summary> <command>
hermoso work block <full-context> <work-item-id> <evidence-id> <reason> <command>
hermoso construction integrate <full-context> [work-item-ids...]
hermoso result put <full-context> <result-path>
hermoso resume <full-context>
```

(Where `<full-context>` = `<project-id> <feature-id> <run-id> <repo>`)

### Construction Round Lifecycle

A construction round is the unit of work between design approval and verification. If verification fails and remediation is needed, a new round starts. Each round is numbered (1, 2, 3, ...).

```
Round N:
  1. graph put          — persist work graph (DAG of work items)
  2. construction prepare — compile work graph into Kanban cards, create git worktrees
  3. construction ready  — list cards that are ready to execute (parents satisfied)
  4. work start/complete/block — execute each work item
  5. construction integrate — merge all work-item branches into feature branch
  6. result put          — persist construction phase result
  → proceeds to verification
```

### Step-by-Step

**`graph put`** — Persist the work graph:

```
app.runGraph()
  → service.PutGraph(ctx, execution, data)
      → json.Unmarshal into domain.WorkGraph
      → graph.Validate()                — check DAG, no cycles, valid work items
      → compute hash (sha256 of canonical graph JSON)
      → store.UpdateRun:
          → append new ConstructionState to run.ConstructionRounds
          → set ConstructionState.Graph = graph
          → set ConstructionState.Hash = hash
          → set ConstructionState.Number = len(rounds)
```

**`construction prepare`** — Create the execution plan:

```
app.runConstruction("prepare")
  → profilePath from arg or store.ResolveProfile(ctx, execution)
  → service.Prepare(ctx, execution, profilePath)
      → store.ReadRun → get current run + latest construction round
      → LoadProfile(profilePath)         — read Hermoso profile YAML
      → dispatch.Compile(graph, config)  — compile work graph into Kanban cards
          → for each work item in topological order:
              → create Card with title, body, parents, workspace info
              → assign profile, skills, lifecycle commands
              → compute idempotency key (sha256 of context + round + work item)
          → return Plan{Cards: []Card}
      → repository.Manager: create git worktree per work item
          → for each card:
              → Manager.CreateWorktree(feature, workspace-id, parent-branch)
              → worktree branched from feature branch (or parent work item's branch)
      → store.UpdateRun:
          → set ConstructionState.Items = work states with workspaces
          → set ConstructionState.ProfilePath = resolved profile path
```

**`construction ready`** — List executable cards:

```
app.runConstruction("ready")
  → service.Ready(ctx, execution)
      → read run → get current construction round
      → for each work item:
          → check all parent work items are completed
          → check this item is not already started/completed/blocked
          → if all parents done and item is pending → it's ready
      → return list of ready cards
```

**`work start`** — Begin a work item:

```
app.runWork("start")
  → service.StartWork(ctx, execution, workItemID)
      → store.UpdateRun:
          → find work item in current round
          → set WorkState.Status = WorkStarted
          → set WorkState.StartedAt = now
          → synchronize parent work items (ensure they're marked started too)
```

**`work complete`** — Finish a work item successfully:

```
app.runWork("complete")
  → build domain.Evidence{Context, ID, Kind: "command", Command, Summary, RecordedAt}
  → service.FinishWork(ctx, execution, workItemID, WorkCompleted, evidence, "")
      → store.UpdateRun:
          → set WorkState.Status = WorkCompleted
          → set WorkState.EndedAt = now
          → append evidence to WorkState.Evidence
          → check if all work items complete → if so, run is ready for integration
```

**`work block`** — Mark a work item as blocked:

```
app.runWork("block")
  → build domain.Evidence with blocker reason as summary
  → service.FinishWork(ctx, execution, workItemID, WorkBlocked, evidence, reason)
      → store.UpdateRun:
          → set WorkState.Status = WorkBlocked
          → set WorkState.Blocker = reason
          → set WorkState.EndedAt = now
          → append evidence
          → set run.Status = StatusBlocked
```

**`construction integrate`** — Merge work-item branches:

```
app.runConstruction("integrate")
  → service.Integrate(ctx, execution, rest)
      → read run → get current construction round
      → for each work item (or specified subset):
          → Manager.IntegrateWorktree(feature, workspace-id)
              → merge work-item branch into feature branch
              → record the integration commit
      → store.UpdateRun:
          → set ConstructionState.IntegratedAt = now
          → set ConstructionState.IntegratedFeatureCommit = commit
          → set ConstructionState.IntegratedLeaves = [{id, branch, commit}, ...]
          → set run.Phase = PhaseVerification
          → set run.Status = StatusAwaitingVerification
```

**`result put`** — Persist construction result:

```
app.runResult()
  → service.PutResult(ctx, execution, data)
      → json.Unmarshal into domain.PhaseResult
      → store.UpdateRun:
          → set ConstructionState.Result = &result
```

**`resume`** — Recover from interruption:

```
app.runResume()
  → service.Resume(ctx, execution)
      → read run → determine current phase + status
      → return current state (the run is always durable on disk)
      → Hermes TUI uses this to decide which command to issue next
```

### Key Invariants

- Work items form a DAG (directed acyclic graph). Cycles are rejected at validation.
- Each work item gets its own git worktree, branched from its parent's branch (or the feature branch for root items).
- Integration merges all work-item branches back into the feature branch in topological order.
- The construction round number increments on each remediation cycle.

---

## 7. Phase 4: Verification

### Commands

```
hermoso verification run <full-context>
hermoso verification judge <full-context> <judgments-path>
hermoso verification remediate <full-context> <spec-path>
hermoso verification resolve <full-context> <resolutions-path>
```

### Verification Loop

Verification is a loop that can cycle back to construction:

```
                    ┌──────────────────┐
                    │ verification run │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │ verification     │
                    │ judge            │
                    └────────┬─────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
     PASS     │    REMEDIATION   │   SURFACES     │
              │              │              │
    ┌─────────▼────┐  ┌──────▼───────┐  ┌──▼──────────┐
    │ publish      │  │ remediate    │  │ resolve     │
    │ Gherkin      │  │ (new round)  │  │ (surfaces)  │
    └─────────┬────┘  └──────┬───────┘  └──┬──────────┘
              │              │              │
    awaiting_release   back to        awaiting_release
                       construction
                       (round N+1)
```

### Step-by-Step

**`verification run`** — Execute the verification contract:

```
app.runVerificationContract("run")
  → service.RunVerification(ctx, execution)
      → read run → get design verification contract + latest construction
      → read sealed artifacts (Gherkin files) from state
      → verify artifact content hashes match contract
      → create VerificationAttempt{
          Number: len(attempts) + 1,
          CandidateCommit: construction.IntegratedFeatureCommit,
          ...
      }
      → execute verification (run the Gherkin scenarios against the feature branch)
      → produce VerificationReport with verdict (pass/fail/blocked)
      → store.UpdateRun:
          → append attempt to run.VerificationAttempts
          → if verdict == pass:
              → set run.Status = StatusAwaitingJudgment
          → if verdict == fail:
              → set run.Status = StatusAwaitingJudgment (judgments may require remediation)
          → if verdict == blocked:
              → set run.Status = StatusBlocked
              → set run.VerificationBlocker = reason
```

**`verification judge`** — Apply skill judgments:

```
app.runVerificationContract("judge")
  → ReadFile(judgments-path)
  → json.Unmarshal into []domain.SkillJudgment
  → service.JudgeVerification(ctx, execution, judgments)
      → read run → get latest verification attempt
      → for each judgment:
          → match to a verification surface or scenario
          → apply verdict (pass/fail/needs_remediation)
      → compute overall verdict from judgments
      → store.UpdateRun:
          → set attempt.Report = updated report
          → compute report hash
          → if overall pass:
              → call publishPassingGherkin() (see below)
          → if needs remediation:
              → set run.Status = StatusAwaitingRemediation
```

**`publishPassingGherkin`** (internal, called on verification pass):

```
service.publishPassingGherkin(ctx, execution, run, attempt)
  → read construction round → get feature workspace
  → Manager.CurrentCommit(feature) — get current HEAD of feature branch
  → read sealed artifacts (approved Gherkin content)
  → for each Gherkin artifact in the verification contract:
      → verify content hash matches (tamper detection)
      → verify publication path stays within feature worktree (no escape)
      → write Gherkin file to feature worktree
  → Manager.CommitAllowedFiles(feature, publishedPaths, commit message)
      → commit only the Gherkin files (not other changes)
      → commit message includes: feature ID, run ID, attempt number, package hash
  → model.Build(ctx, ...) — rebuild repository model at the new commit
  → model.AddPublishedScenarios(...) — annotate model with published Gherkin
  → store.PutModelSnapshot(ctx, snapshot) — persist updated model
  → store.UpdateRun:
      → set run.Publication = &GherkinPublication{
          Attempt, VerifiedCommit, PublicationCommit, Model, PublishedPaths, PublishedAt
        }
      → set run.Phase = PhaseVerification (stays here until release)
      → set run.Status = StatusAwaitingRelease
      → clear run.VerificationBlocker
```

**Publication rollback:** If any step fails after files have been written, the `defer` function restores original file contents (or removes newly created files). This ensures atomic publication.

**`verification remediate`** — Start a new construction round:

```
app.runVerificationContract("remediate")
  → ReadFile(spec-path)
  → service.PutRemediation(ctx, execution, data)
      → json.Unmarshal into domain.RemediationSpec
      → store.UpdateRun:
          → append new ConstructionState to run.ConstructionRounds
          → set new round.Number = len(rounds)
          → set new round.Kind = "remediation"
          → set new round.Remediation = &spec
          → set new round.SourceHash = hash of previous construction
          → set run.Phase = PhaseConstruction
          → set run.Status = StatusInProgress
      → operator now runs `graph put` → `construction prepare` → ... for the new round
```

**`verification resolve`** — Apply surface resolutions (alternative to remediation):

```
app.runVerificationContract("resolve")
  → ReadFile(resolutions-path)
  → json.Unmarshal into []domain.SurfaceResolution
  → service.PutSurfaceResolutions(ctx, execution, resolutions)
      → read run → get latest verification attempt
      → for each resolution:
          → match to a verification surface
          → apply resolution (accept, reject, or override)
      → if all surfaces resolved as passing:
          → call publishPassingGherkin()
      → store.UpdateRun with updated report
```

### Key Invariants

- Verification artifacts are sealed (content-hashed) at design time. Tampering between design and verification is detected.
- Publication paths are confined to the feature worktree. Path traversal (../../etc/passwd) is rejected.
- The publication commit is distinct from the candidate commit (they must differ).
- Gherkin publication creates a fresh repository model snapshot at the new commit.
- Remediation creates a brand-new construction round — the operator re-runs the full construction cycle.

---

## 8. Phase 5: Release

### State Transition

Once verification passes and Gherkin is published:

```
run.Phase = PhaseVerification
run.Status = StatusAwaitingRelease
```

Release is the terminal phase. The transition to `released` is the final state change:

```
StatusAwaitingRelease → StatusReleased
run.Phase = PhaseRelease
```

At this point:
- The feature branch contains the verified code + published Gherkin scenarios.
- The repository model has been refreshed at the publication commit.
- The run is complete and durable on disk.

The operator merges the feature branch into the default branch (or opens a PR) using standard git tooling. Hermoso does not perform the merge to main — it verifies and publishes, then hands off.

---

## 9. Cross-Cutting: Repository Model

### Commands

```
hermoso model build <project-id> <repo> [--revision <git-ref>] [--scip <path>]
hermoso model status <project-id> <repo>
hermoso model query <project-id> <repo> <orientation|task|impact|evidence> [args...] [--budget <bytes>]
hermoso model explain <project-id> <repo> <node-id>
```

### What It Does

The repository model is a SCIP-based (Source Code Intelligence Protocol) code graph that Hermoso builds from the target repository. It provides:

- **Orientation** — "What does this codebase do?" High-level summary for an agent entering the codebase.
- **Task** — "Where should I make this change?" Finds relevant files/nodes for a described task.
- **Impact** — "What will break if I change this?" Traces dependencies from a node.
- **Evidence** — "Explain this node." Full detail for a specific code node.

The model is built at a specific git revision and stored as a JSON snapshot under `.hermoso/models/`. It's rebuilt after Gherkin publication to incorporate the new verified scenarios.

**Model freshness:** `model status` checks whether the model's source revision matches the current HEAD. If they differ, the model is stale and should be rebuilt.

---

## 10. Cross-Cutting: Git Worktree Management

The `repository.Manager` (in `internal/repository/manager.go`) handles all git operations:

### Worktree Lifecycle

```
Feature branch (e.g. feature/add-auth)
  │
  ├── Worktree A (work item 1) ── branched from feature branch
  │     └── commits for work item 1
  │
  ├── Worktree B (work item 2) ── branched from feature branch (or from A if B depends on A)
  │     └── commits for work item 2
  │
  └── Worktree C (work item 3) ── branched from feature branch
        └── commits for work item 3
```

**Creation:** `Manager.CreateWorktree(feature, workspaceID, parentBranch)` creates a git worktree at `.hermoso/worktrees/<workspace-id>/`, branched from the specified parent.

**Integration:** `Manager.IntegrateWorktree(feature, workspaceID)` merges the work-item branch back into the feature branch.

**Clean check:** `Manager.WorktreeClean(feature)` verifies the worktree has no uncommitted changes before publication.

**Allowed-files commit:** `Manager.CommitAllowedFiles(feature, paths, message)` commits only the specified files, leaving other changes in the working tree. Used by Gherkin publication to commit only .feature files.

**Validate allowed commit:** `Manager.ValidateAllowedCommit(feature, commit, paths)` verifies that a specific commit only touched the allowed files. Used when resuming publication.

### Key Methods

| Method                      | Purpose                                          |
|-----------------------------|--------------------------------------------------|
| `CurrentCommit(feature)`    | Get HEAD commit of the feature worktree          |
| `CreateWorktree(...)`       | Create a git worktree for a work item            |
| `IntegrateWorktree(...)`    | Merge a work-item branch into the feature branch |
| `WorktreeClean(feature)`    | Check if worktree has no uncommitted changes     |
| `CommitAllowedFiles(...)`   | Commit only specified files                      |
| `ValidateAllowedCommit(...)`| Verify a commit only touched specified files     |

---

## 11. Cross-Cutting: Dispatch and Kanban

### What It Does

The `dispatch` package (in `internal/dispatch/dispatch.go`) compiles a work graph into Kanban-ready card specifications. This is the bridge between Hermoso's internal work graph and an external Kanban system (like Hermes Kanban).

### Compilation Flow

```
dispatch.Compile(graph, config) → Plan
  → validate graph (DAG, no cycles)
  → validate config (round, prepared workspaces, profile)
  → topological sort of work items
  → for each work item:
      → create Card{
          Identity:     WorkItemIdentity{context, round, work-item-id},
          Title:        work item title,
          Body:         work item body / instructions,
          Parents:      parent work items (for dependency tracking),
          Tenant:       kanban tenant (from project ID),
          Priority:     from config,
          WorkspaceKind: "worktree",
          WorkspacePath: path to git worktree,
          AssignedProfile: resolved Hermes profile,
          ForcedSkills:   skills bound to this work item,
          LifecycleCommands: {prepare, execute, validate} — hermoso CLI commands,
          AcceptanceCriteria: from design,
          BaseModel:     from feature design,
          IdempotencyKey: sha256(context + round + work item),
        }
  → return Plan{Context, Round, Cards}
```

### Card Structure

Each card is self-contained — it carries everything an external agent needs to execute the work item:

- **Workspace** — which git worktree to work in
- **Profile** — which Hermes profile (model, gateway, skills) to use
- **Skills** — which Hermoso skills to load
- **Lifecycle commands** — the exact `hermoso` CLI commands for prepare/execute/validate
- **Acceptance criteria** — what must be true for the work item to be complete
- **Base model** — the repository model reference for code intelligence

The `construction ready` command returns the subset of cards whose parent work items are all completed — these are the ones ready to be dispatched to agents.

---

## 12. Command Reference

### Full Command Tree

```
hermoso
  init <repo> [--profile <path>]                    Initialize project state
  start <feature-id> <repo> [--profile <path>]      Start a new run
  status <repo>                                     Show project + runs
  context <project> <feature> <run> <repo>          Resolve execution context

  design
    put <full-context> <path>                       Persist feature design

  verification
    put <full-context> <path>                       Persist verification contract
    run <full-context>                              Execute verification
    judge <full-context> <judgments-path>           Apply skill judgments
    remediate <full-context> <spec-path>            Start remediation round
    resolve <full-context> <resolutions-path>       Apply surface resolutions

  approve
    design <full-context> <revision> <hash> <actor> [comment]   Approve design package

  graph
    put <full-context> <path>                       Persist work graph

  construction
    prepare <full-context> [profile-path]           Compile work graph → cards + worktrees
    ready <full-context>                            List ready-to-execute cards
    integrate <full-context> [work-item-ids...]     Merge work-item branches

  task
    bind <full-context> <work-item-id> <task-id>    Bind external Kanban task

  work
    start <full-context> <work-item-id>             Start a work item
    complete <full-context> <work-item-id> <evidence-id> <summary> <command>   Complete work
    block <full-context> <work-item-id> <evidence-id> <reason> <command>       Block work

  result
    put <full-context> <path>                       Persist construction result

  resume <full-context>                             Resume from durable state

  model
    build <project> <repo> [--revision <ref>] [--scip <path>]   Build repo model
    status <project> <repo>                         Check model freshness
    query <project> <repo> <mode> [args...] [--budget <bytes>]  Query model
    explain <project> <repo> <node-id>              Explain a node

  schema <kind>                                     Get JSON schema for contract kind
  validate <kind> <path> <full-context>             Validate a contract file

  version                                           Print version
  help                                              Print usage
```

Where `<full-context>` = `<project-id> <feature-id> <run-id> <repo-path>`

All commands support `--json` for machine-readable output.

### Typical Lifecycle Sequence

For a single feature with no remediation:

```
1.  hermoso init .                                       → project created
2.  hermoso start feature-001 .                          → run created
3.  hermoso design put <ctx> design.json                 → feature design persisted
4.  hermoso verification put <ctx> verification.json     → verification contract persisted
5.  hermoso approve design <ctx> <rev> <hash> marcos     → design approved, phase → construction
6.  hermoso graph put <ctx> graph.json                   → work graph persisted
7.  hermoso construction prepare <ctx>                   → cards compiled, worktrees created
8.  hermoso construction ready <ctx>                     → list ready cards
9.  hermoso work start <ctx> <item-1>                    → begin work item 1
10. hermoso work complete <ctx> <item-1> <eid> "done" "pytest"  → finish item 1
11. hermoso work start <ctx> <item-2>                    → begin work item 2 (was waiting on 1)
12. hermoso work complete <ctx> <item-2> <eid> "done" "pytest"  → finish item 2
13. hermoso construction integrate <ctx>                 → merge branches, phase → verification
14. hermoso result put <ctx> result.json                 → construction result persisted
15. hermoso verification run <ctx>                       → execute verification
16. hermoso verification judge <ctx> judgments.json      → apply skill judgments
    (if pass → Gherkin published automatically)
17. (feature branch now has verified code + Gherkin)
    → operator merges feature branch to main or opens PR
```

With remediation, steps 6-16 repeat as a new construction round (round 2, 3, ...) until verification passes.

---

## Appendix: Layer Responsibilities

| Layer              | Package                    | Responsibility                                    |
|--------------------|----------------------------|---------------------------------------------------|
| CLI                | `internal/app`             | Arg parsing, command dispatch, output formatting  |
| Workflow           | `internal/workflow`        | Phase orchestration, state transitions            |
| State              | `internal/state`           | File I/O, locking, atomic writes, project/run CRUD|
| Domain             | `internal/domain`          | Pure types, validation, hashing, phase/status rules|
| Repository         | `internal/repository`      | Git worktree creation, branch merging, commits    |
| Dispatch           | `internal/dispatch`        | Work graph → Kanban card compilation              |
| Verification       | `internal/verification`    | Verification contract validation                  |
| Model              | `internal/model`           | SCIP-based code intelligence, querying            |
| Contracts          | `internal/contracts`       | JSON schema generation + validation               |
| Digest             | `internal/digest`          | SHA-256 hashing utilities                         |
