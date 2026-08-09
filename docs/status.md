# Implementation status

Status reflects the repository as of 2026-08-02.

## Available

- CLI commands exactly as reported by `hermoso help`: `help`, `version`,
  `schema`, `validate`, `init`, `start`, `status`, `context`, `design put`,
  `approve design`, `graph put`, `construction prepare`, `construction ready`,
  `construction integrate`, `task bind`, `work start`, `work complete`,
  `work block`, `result put`, and `resume`.
- Source-controlled Hermes controller, design, and construction skills,
  configurable default phase/profile bindings, and a non-mutating Hermes
  readiness doctor.
- Stable text/JSON output conventions and process exit codes.
- Versioned `hermoso-context/v1` identity embedded in the schemas for
  `feature-design`, `work-graph`, and `phase-result`.
- Domain validation for project identity, designs, acyclic work graphs, phase
  results, evidence, exact-revision approvals, external task bindings, and
  lifecycle transitions.
- Design and construction states, including blocked-to-in-progress resume
  transitions, in the domain model.
- Internal state storage for Git repository discovery, project
  initialization/loading, run creation/update, process locking, atomic JSON
  writes, and local exclusion of `.hermoso` from Git.
- Explicit repository/run validation for CLI operations, context-bound task
  bindings, project-derived Kanban tenants, and full-context dispatch
  idempotency/card instructions.
- Unit, Git/worktree integration, and deterministic fake-Kanban end-to-end
  tests covering design through `awaiting_verification`, parallel roots,
  fan-in, task binding, blockers/resume, idempotency, and context isolation.
- Context-bound design and work-graph persistence with canonical SHA-256
  hashes, revision checks, and exact design approval.
- Construction preparation through the repository manager, deterministic
  create-ready card emission, external Kanban task binding, parent
  synchronization, work evidence/block/resume, branch integration, and
  construction results transitioning to `awaiting_verification`.
- Retry-safe state changes and managed Git operations that never reset dirty or
  conflict-resolution work.

## Scope

The first operational milestone covers **design and construction** through the
Hermes TUI. Verification and release types are reserved in the lifecycle model
but have no commands yet. Hermoso is not currently an autonomous dark-factory
or self-improving Spec Kit runner; documents describing that former direction
are retained only in the [historical archive](archive/README.md).

## Planned (not CLI commands)

- Verification-phase orchestration and evidence verdicts.
- Release approval, promotion, and release evidence.
- Packaging/versioned distribution beyond building the current Go command.

There are no `verify`, `release`, `setup skills`, or profile-management commands
in the current CLI.
