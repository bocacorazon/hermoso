# Usage and workflow

## Build and inspect the current CLI

Hermoso requires Go 1.26 or newer. Repository-model builds use Tree-sitter and
therefore require CGO and a working C compiler.

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
hermoso schema <feature-design|feature-verification-contract|work-graph|phase-result> [--json]
hermoso validate <feature-design|feature-verification-contract|work-graph|phase-result> <path> <project-id> <feature-id> <run-id> <repository> [--json]
hermoso init <repository> [--profile <path>] [--json]
hermoso start <feature-id> <repository> [--profile <path>] [--json]
hermoso status <repository> [--json]
hermoso context <project-id> <feature-id> <run-id> <repository> [--json]
hermoso model build <project-id> <repository> [--revision <commit>] [--scip <path>] [--json]
hermoso model status <project-id> <repository> [--json]
hermoso model query <project-id> <repository> <orientation|task|impact|evidence> [query-or-node ...] [--budget <bytes>] [--json]
hermoso model explain <project-id> <repository> <node-id> [--budget <bytes>] [--json]
hermoso design put <project-id> <feature-id> <run-id> <repository> <path> [--json]
hermoso verification put <project-id> <feature-id> <run-id> <repository> <path> [--json]
hermoso approve design <project-id> <feature-id> <run-id> <repository> <package-revision> <package-hash> <actor> [comment] [--json]
hermoso graph put <project-id> <feature-id> <run-id> <repository> <path> [--json]
hermoso construction prepare <project-id> <feature-id> <run-id> <repository> [profile-path] [--json]
hermoso construction ready <project-id> <feature-id> <run-id> <repository> [--json]
hermoso task bind <project-id> <feature-id> <run-id> <repository> <work-item-id> <task-id> [--json]
hermoso work start <project-id> <feature-id> <run-id> <repository> <work-item-id> [--json]
hermoso work complete <project-id> <feature-id> <run-id> <repository> <work-item-id> <evidence-id> <summary> <command> [--json]
hermoso work block <project-id> <feature-id> <run-id> <repository> <work-item-id> <evidence-id> <reason> <command> [--json]
hermoso resume <project-id> <feature-id> <run-id> <repository> [--json]
hermoso construction integrate <project-id> <feature-id> <run-id> <repository> [check ...] [--json]
hermoso result put <project-id> <feature-id> <run-id> <repository> <phase-result-path> [--json]
hermoso verification run <project-id> <feature-id> <run-id> <repository> [--json]
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
only current commands; release, skill setup, and profile management are not
available.

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

## Profile resolution

Hermoso supports per-repo and per-run profile configuration. A profile is a
JSON file that maps phases to Hermes profile names and model settings.

**Per-repo profile.** Pass `--profile <path>` at `init` time. The path is
persisted in `.hermoso/project.json` and becomes the default for all runs in
that repository:

```sh
hermoso init /path/to/repository --profile /path/to/profile.json --json
```

**Per-run override.** Pass `--profile <path>` at `start` time. The override is
stored on the run record and takes precedence over the repo-level default for
that run only:

```sh
hermoso start feature-id /path/to/repository --profile /path/to/override.json --json
```

**Resolution chain.** When `construction prepare` is called without an explicit
profile path argument, Hermoso resolves the profile in this order:

1. Run-level `profile_path` (set via `start --profile`)
2. Project-level `profile_path` (set via `init --profile`)
3. Error: `no profile configured for project or run`

When a profile path is passed directly to `construction prepare`, it takes
precedence over both run-level and project-level settings.

## Project and run workflow

Project initialization, run creation, and status inspection are available:

```sh
hermoso init /path/to/repository [--profile /path/to/profile.json] --json
hermoso start feature-id /path/to/repository [--profile /path/to/override.json] --json
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
3. **Model and design.** Build/query a fresh spine snapshot. Persist the v2
   design with stable requirements, criteria, vocabulary, and surfaces.
4. **Verification contract.** A separate author ingests the hidden hybrid
   contract and assets with `verification put`.
5. **Approve.** `approve design` records the exact complete package
   revision/hash.
6. **Construct.** Persist the traceable acyclic graph with `graph put`, then use
   `construction prepare` to create managed workspaces and compile the plan.
7. **Dispatch.** `construction ready` emits only create-ready card specs and
   excludes hidden verification content.
   Hermes creates them externally with each card's idempotency key and records
   returned IDs immediately with `task bind`.
8. **Collect.** Results and evidence are validated and attached to the run.
   Dependent tasks become ready only after their parents complete.
9. **Complete or block.** Integrate branches, then persist the construction
   result. A completed result transitions to `awaiting_verification`.
10. **Verify.** `verification run` evaluates the exact candidate read-only and
   persists its report. One first-failure remediation round is automatic; a
   second failure blocks.
11. **Publish.** A pass commits only approved Gherkin, refreshes the spine with
   stable scenario links, and transitions to `awaiting_release`.

## Recovery, blockers, and safe retries

A blocked `phase-result` must contain at least one unresolved blocker. Hermes
shows those blockers to the developer; Hermoso preserves the run state and does
not treat a blocked phase as completed. After the external condition is fixed,
`resume` transitions blocked construction back to in-progress and continues
from durable state rather than creating a replacement run. For a blocked or
inconclusive verifier infrastructure incident, it archives the report and
returns the same semantic verification attempt to `awaiting_verification`.
Failed behavioral judgments remain subject to the one-remediation-round rule.

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
7. For a blocked or inconclusive verification incident, correct the external
   condition, run `resume`, then retry `verification run`. The incident report
   remains in run history and does not consume attempt 1 or 2.
8. If post-pass publication is blocked, correct the external condition and run
   `resume`. Hermoso validates any existing publication commit against the
   verified candidate and allowlist before completing the spine refresh.
9. Treat corrupt/incompatible state as an operator incident. Preserve the
   repository and `.hermoso` directory for diagnosis rather than reinitializing.

## Verification and release boundary

Verification is implemented through `verification run`; see
[Feature verification contracts](verification-contracts.md). Release
promotion remains future work and begins only from `awaiting_release`.
