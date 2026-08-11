# Architecture

## Product boundary

Hermoso uses three cooperating components:

1. **Hermes TUI — primary UX.** The developer stays in Hermes to describe a
   feature, review design output, resolve blockers, and observe progress.
   Hermes skills perform the reasoning and present human decisions.
2. **Hermoso Go CLI — deterministic control plane.** `hermoso` validates
   contracts, owns durable project/run state, enforces lifecycle transitions
   and exact-revision approvals, records evidence and external task bindings,
   and provides stable JSON operations for skills.
3. **Hermes Kanban — dispatch and visibility.** Construction work items are
   represented as Kanban tasks. Hermes dispatches workers and manages the
   board; Hermoso records the binding between a work item and its external task
   rather than implementing another queue or board.

Hermoso is not a second chat interface, an LLM client, a code-writing agent, or
a replacement for Hermes Kanban. It does not install a project-specific agent
framework into target repositories.

## Phase contracts

Skills and the CLI communicate through versioned JSON contracts:

- **`hermoso-context/v1`**: the canonical project, feature, run, and repository
  identity. It is embedded in every phase contract, approval, evidence record,
  task binding, dispatch identity, and card body. Identity is never inferred
  from cwd or conversation.

- **`feature-design` v2**: stable requirements and acceptance criteria,
  business vocabulary, constraints, decisions, and existing/planned interaction
  surfaces bound to an immutable repository-model snapshot.
- **`feature-verification-contract`**: hidden hybrid BDD, deterministic,
  property, and rubric judgments plus hash-locked verifier assets and complete
  requirement/surface traceability.
- **`work-graph`**: an acyclic construction graph. Each item declares its
  visible requirement, criterion, and surface IDs in addition to its prompt,
  local criteria, dependencies, worker bindings, commands, and budget.
- **`phase-result`**: a terminal result for a phase, with input/output
  references, decisions, warnings, blockers, evidence, and completion time.

One design-package approval binds the design, verification contract, sealed
artifact root, and model snapshot hashes. Editing any input invalidates it.

Each project has a deterministic Kanban tenant derived from `project_id`.
Dispatch idempotency includes the complete context, and card lifecycle commands
refresh context against the explicit run and canonical repository before work.
Managed branch names and worktree paths also include `run_id`, so two runs of
the same feature can coexist without sharing a feature, item, or integration
branch. Separate repositories provide the project boundary; feature and run
identity provide isolation inside one repository.

The lifecycle models `design`, ordered initial/remediation construction rounds,
up to two verification attempts, Gherkin publication, and `release`.

## Repository knowledge spine

`.hermoso/model` contains immutable, content-addressed repository snapshots.
Git and manifest inventory, Tree-sitter structure for Go/JavaScript/TypeScript/
TSX/Python, optional SCIP data, testing surfaces, commands, interfaces,
vocabulary, provenance, and bounded views give design and verification a common
repository model. Contracts cite exact snapshot/source/vocabulary hashes;
stale snapshots are rejected.

## `.hermoso` state

Durable state lives in the target repository and contains state, not copied
orchestration assets. The state store persists the complete run atomically in `run.json`, including
the contracts, hashes, approval, workspaces, bindings, evidence, blockers, and
construction result:

```text
.hermoso/
├── model/
│   ├── current.json
│   └── snapshots/<snapshot-id>/
├── artifacts/<run-id>/<root-hash>/
├── verification/<run-id>/attempt-<n>/assets/
├── project.json
├── state.lock
├── runs/
│   └── <run-id>/
│       └── run.json
└── worktrees/
    └── <run-id>/
```

The current state package discovers and validates Git repository identity,
locks updates, writes JSON atomically, and adds `/.hermoso/` to the repository's
local Git exclude file. Managed branches/worktrees are reused only after
ownership and cleanliness checks; user work is never reset.

`.hermoso` JSON is an internal persistence boundary, not a user-editable API.
Recovery must go through `status`, `context`, `resume`, and an idempotent retry
of the failed command. Skills and profiles remain source-controlled in this
repository and are loaded by Hermes; target repositories contain only state and
managed Git work.

## Control flow

```text
Hermes conversation
  -> design skill
  -> Hermoso validates feature-design against a fresh spine snapshot
  -> verification author seals the hidden contract and assets
  -> developer approves the exact atomic design package
  -> construction skill creates work-graph
  -> Hermes Kanban creates and dispatches tasks
  -> workers return evidence/results
  -> Hermoso integrates leaves and validates the construction result
  -> isolated read-only verification attempt
     -> pass: publish approved Gherkin and refresh spine
     -> first fail: one sanitized remediation round and second attempt
     -> second fail or invalid attempt: blocked
```

Skills own judgment and presentation. Hermoso owns deterministic invariants.
Kanban owns dispatch. Keeping those boundaries explicit avoids duplicating
Hermes while making runs inspectable and resumable.

Verification commands and reports are implemented. Release promotion remains a
separate future boundary and may act only after `awaiting_release`.
