# Implementation status

Status reflects the repository as of 2026-08-02.

## Available

- CLI commands exactly as reported by `hermoso help`: `help`, `version`,
  `schema`, `validate`, `init`, `start`, `status`, `context`, `model build`,
  `model status`, `model query`, `model explain`, `design put`,
  `verification put`, `verification run`,
  `approve design`, `graph put`, `construction prepare`, `construction ready`,
  `construction integrate`, `task bind`, `work start`, `work complete`,
  `work block`, `result put`, and `resume`.
- Source-controlled Hermes controller, design, verification-author,
  construction, and verification skills,
  configurable default phase/profile bindings, and a non-mutating Hermes
  readiness doctor.
- Stable text/JSON output conventions and process exit codes.
- Versioned `hermoso-context/v1` identity embedded in v2 feature design,
  verification contract, work graph, and phase result schemas.
- Domain validation for project identity, designs, acyclic work graphs, phase
  results, evidence, exact-revision approvals, external task bindings, and
  lifecycle transitions.
- Immutable repository-model snapshots with Git/manifest facts, Tree-sitter
  indexing for Go/JavaScript/TypeScript/TSX/Python, optional SCIP enrichment,
  bounded queries, freshness, provenance, testing, interfaces, and vocabulary.
- Atomic design packages binding visible design, hidden hybrid verification
  contract, sealed assets, model snapshot, exact approval, and invalidation.
- Ordered construction rounds and up to two persisted verification attempts.
- Internal state storage for Git repository discovery, project
  initialization/loading, run creation/update, process locking, atomic JSON
  writes, and local exclusion of `.hermoso` from Git.
- Explicit repository/run validation for CLI operations, context-bound task
  bindings, project-derived Kanban tenants, and full-context dispatch
  idempotency/card instructions.
- Unit, Git/worktree integration, and deterministic fake-Kanban end-to-end
  tests covering design through Gherkin publication, parallel roots,
  fan-in, task binding, blockers/resume, idempotency, and context isolation.
- Context-bound design and work-graph persistence with canonical SHA-256
  hashes, revision checks, and exact design approval.
- Construction preparation through the repository manager, deterministic
  create-ready card emission, external Kanban task binding, parent
  synchronization, work evidence/block/resume, branch integration, and
  construction results transitioning to `awaiting_verification`.
- Retry-safe state changes and managed Git operations that never reset dirty or
  conflict-resolution work.
- Read-only isolated verification with command/evidence reports, candidate
  mutation detection, surface resolution, one sanitized remediation round,
  second-failure blocking, allowlisted Gherkin publication, and post-pass spine
  scenario promotion.

## Scope

The operational milestone covers **design, construction, and verification**
through the Hermes TUI. Release promotion remains outside the current command
surface. Hermoso is not an autonomous dark-factory
or self-improving Spec Kit runner; documents describing that former direction
are retained only in the [historical archive](archive/README.md).

## Planned (not CLI commands)

- Release approval, promotion, and release evidence.
- Packaging/versioned distribution beyond building the current Go command.

There are no release, skill-setup, or profile-management commands in the
current CLI.
