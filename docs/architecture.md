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

- **`feature-design`**: objective, acceptance criteria, constraints, decisions,
  non-goals, complexity, architecture/interface notes, and research references.
  A valid design has no unresolved questions.
- **`work-graph`**: an acyclic construction graph. Each item declares its
  prompt, acceptance criteria, parent items, worker profile, ordered skills,
  expected changed surfaces, validation commands, and optional runtime budget.
- **`phase-result`**: a terminal result for a phase, with input/output
  references, decisions, warnings, blockers, evidence, and completion time.

Contract references include a revision and SHA-256 content hash. Design
approval is therefore attached to the exact reviewed revision and hash; editing
the design invalidates the old approval.

Each project has a deterministic Kanban tenant derived from `project_id`.
Dispatch idempotency includes the complete context, and card lifecycle commands
refresh context against the explicit run and canonical repository before work.
Managed branch names and worktree paths also include `run_id`, so two runs of
the same feature can coexist without sharing a feature, item, or integration
branch. Separate repositories provide the project boundary; feature and run
identity provide isolation inside one repository.

The lifecycle models `design`, `construction`, `verification`, and `release`.
The approved first scope exposes the design and construction workflow.
Verification and release are reserved for later commands.

## `.hermoso` state

Durable state lives in the target repository and contains state, not copied
orchestration assets. The state store persists the complete run atomically in `run.json`, including
the contracts, hashes, approval, workspaces, bindings, evidence, blockers, and
construction result:

```text
.hermoso/
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
  -> Hermoso validates and persists feature-design
  -> developer approves exact design revision
  -> construction skill creates work-graph
  -> Hermes Kanban creates and dispatches tasks
  -> workers return evidence/results
  -> Hermoso integrates leaves and validates the construction result
  -> run waits at awaiting_verification
```

Skills own judgment and presentation. Hermoso owns deterministic invariants.
Kanban owns dispatch. Keeping those boundaries explicit avoids duplicating
Hermes while making runs inspectable and resumable.

Future verification will consume the immutable construction result and evidence
without weakening the context boundary. Future release will act only on a
verified run and will own promotion/release evidence; neither phase has CLI
commands today.
