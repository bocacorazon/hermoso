# Usage and workflow

## Build and inspect the current CLI

Hermoso currently requires Go 1.26 or newer.

```sh
go test ./...
go build -o ./hermoso ./cmd/hermoso
install -m 0755 ./hermoso "$HOME/.local/bin/hermoso"
go run ./cmd/hermoso help
go run ./cmd/hermoso version --json
scripts/hermoso-doctor.sh --static
```

### Available commands

```sh
hermoso help
hermoso version [--json]
hermoso schema <feature-design|work-graph|phase-result> [--json]
hermoso validate <feature-design|work-graph|phase-result> <path> <project-id> <feature-id> <run-id> <repository> [--json]
hermoso init <repository> [--json]
hermoso start <feature-id> <repository> [--json]
hermoso status <repository> [--json]
hermoso context <project-id> <feature-id> <run-id> <repository> [--json]
hermoso design put <project-id> <feature-id> <run-id> <repository> <path> [--json]
hermoso approve design <project-id> <feature-id> <run-id> <repository> <revision> <hash> <actor> [comment] [--json]
hermoso graph put <project-id> <feature-id> <run-id> <repository> <path> [--json]
hermoso construction prepare <project-id> <feature-id> <run-id> <repository> <profile-path> [--json]
hermoso construction ready <project-id> <feature-id> <run-id> <repository> [--json]
hermoso task bind <project-id> <feature-id> <run-id> <repository> <work-item-id> <task-id> [--json]
hermoso work start <project-id> <feature-id> <run-id> <repository> <work-item-id> [--json]
hermoso work complete <project-id> <feature-id> <run-id> <repository> <work-item-id> <evidence-id> <summary> <command> [--json]
hermoso work block <project-id> <feature-id> <run-id> <repository> <work-item-id> <evidence-id> <reason> <command> [--json]
hermoso resume <project-id> <feature-id> <run-id> <repository> [--json]
hermoso construction integrate <project-id> <feature-id> <run-id> <repository> [check ...] [--json]
hermoso result put <project-id> <feature-id> <run-id> <repository> <phase-result-path> [--json]
```

`schema` emits the live versioned JSON schema. `validate` checks JSON decoding,
rejects unknown fields and extra JSON values, and applies domain validation.
`--json` may appear anywhere in the argument list and is the stable
skill-facing response format. `init` idempotently creates clone-local
`.hermoso` project state, `start` requires initialized state and creates a
pending design-phase run, and read-only `status` returns the project and all
runs. Repository arguments may point at a subdirectory; Hermoso discovers its
canonical Git root. State commands report explicit errors for uninitialized,
corrupt, or repository-incompatible state.

Repository and run arguments are mandatory. Hermoso never derives execution
identity from cwd or conversation. `context` returns the canonical
`hermoso-context/v1` tuple, and `validate` rejects a structurally valid contract
when its context differs from persisted project/run state.

`hermoso help` is authoritative. The command list above intentionally contains
only current commands; `verify`, `release`, `setup skills`, and profile
management are not available.

## One-time skill setup

Hermoso's source-controlled Hermes skills live under `skills/` and are loaded
through Hermes `skills.external_dirs`; they are not copied into target
repositories. See [`skills/README.md`](../skills/README.md) for the manual,
one-time configuration and run `scripts/hermoso-doctor.sh` for non-mutating
profile, skill, Kanban, and gateway readiness checks. There is no
`hermoso setup skills` command.

To replace the checked-out skills, update the single `skills.external_dirs`
entry to the replacement directory, restart the Hermes session, and rerun the
doctor. To replace phase/profile bindings, edit `profiles/default.yaml` or set
`HERMOSO_PROFILE=/absolute/profile.json`. Profiles currently use JSON syntax
even when the filename ends in `.yaml`. Do not copy either asset into the target
repository.

## Project and run workflow

Project initialization, run creation, and status inspection are available:

```sh
hermoso init /path/to/repository --json
hermoso start feature-id /path/to/repository --json
hermoso status /path/to/repository --json
hermoso context <project-id> <feature-id> <run-id> /path/to/repository --json
```

## TUI operation

In the Hermes TUI, invoke the `hermoso` controller skill and provide the
absolute target repository plus a feature objective. The controller must show
the canonical project/feature/run/repository tuple and the exact design
revision/hash before asking for approval. Use Hermes Kanban to inspect cards,
handoffs, and worker status; use CLI JSON state as the authority for lifecycle,
bindings, evidence, and blockers.

The TUI-first flow is:

1. **Initialize (available).** From Hermes, initialize the target project once.
   Hermoso records repository identity under `.hermoso`.
2. **Start.** Start a feature run. Hermoso persists a pending
   design-phase run; Hermes will gather the objective and invoke the design
   skill.
3. **Design.** Persist with `design put`; Hermoso computes the canonical
   content hash. A changed contract must increment its revision.
4. **Approve.** `approve design` records the exact current revision and hash.
5. **Construct.** Persist the acyclic graph with `graph put`, then use
   `construction prepare` to create managed workspaces and compile the plan.
6. **Dispatch.** `construction ready` emits only create-ready card specs.
   Hermes creates them externally with each card's idempotency key and records
   returned IDs immediately with `task bind`.
7. **Collect.** Results and evidence are validated and attached to the run.
   Dependent tasks become ready only after their parents complete.
8. **Complete or block.** Integrate branches, then persist the construction
   result. A completed result transitions to `awaiting_verification`.

## Recovery, blockers, and safe retries

A blocked `phase-result` must contain at least one unresolved blocker. Hermes
shows those blockers to the developer; Hermoso preserves the run state and does
not treat a blocked phase as completed. After the external condition is fixed,
`resume` transitions the same construction run back to in-progress and
continues from durable state rather than creating a replacement run.

Recovery rules:

1. Run `status` and `context`; stop if any project, feature, run, or repository
   field differs from the task/card.
2. Never edit `.hermoso/*.json`, delete managed branches, or reset a managed
   worktree to recover.
3. Retry `design put`, approval, graph put, prepare, task bind, work
   completion, integration, result put, and resume with the same inputs.
   Successful repeats are idempotent.
4. If Kanban creation succeeded but binding failed, bind the returned task ID;
   do not create another card.
5. For a merge conflict, resolve and stage the files in the reported worktree,
   run `resume`, then retry `work start` or `construction integrate`. Hermoso
   continues the recorded merge and does not discard the resolution.
6. For a failed integration check, fix and commit the relevant managed work,
   run `resume`, and retry integration with the same checks.
7. Treat corrupt/incompatible state as an operator incident. Preserve the
   repository and `.hermoso` directory for diagnosis rather than reinitializing.

## Future verification and release

`awaiting_verification` is the delivered construction boundary. Future
verification will evaluate the persisted construction result and evidence, then
produce an explicit verdict. Future release will require that verdict and
record promotion/release evidence. No current command performs either phase.
