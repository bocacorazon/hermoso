# Hermoso

Hermoso is a Go control plane for a **Hermes TUI-first** development workflow.
Hermes skills perform design, construction, and verification reasoning, Hermes
Kanban dispatches workers, and the `hermoso` CLI owns deterministic contracts,
the repository knowledge spine, identity, approvals, Git worktrees, durable
state, evidence, and recovery.

Hermoso is not another chat UI, an LLM client, a worker, or a Kanban
implementation. Skills use the CLI's JSON output; operators may use its text
output for diagnostics.

## Setup

Hermoso requires Go 1.26 or newer.

```sh
go test ./...
go build -o ./hermoso ./cmd/hermoso
install -m 0755 ./hermoso "$HOME/.local/bin/hermoso"
scripts/hermoso-doctor.sh --static
```

Load this checkout's skills through Hermes `skills.external_dirs` and select or
replace the local profile bindings in `profiles/default.yaml`; do not copy skills
or profiles into target repositories. See [skills/README.md](skills/README.md).

## Start from the Hermes TUI

Ask the `hermoso` skill to initialize the target repository and start a feature.
It will use the exact context returned by:

```sh
hermoso init /absolute/repository --json
hermoso start feature-id /absolute/repository --json
hermoso status /absolute/repository --json
hermoso context <project-id> <feature-id> <run-id> /absolute/repository --json
```

The delivered workflow covers spine-grounded design, a hidden hybrid BDD
verification contract, atomic package approval, construction dispatch,
read-only verification, one remediation cycle, and allowlisted Gherkin
publication into a refreshed spine. Release promotion remains future work.

## CLI conventions

- Exit `0` is success, `1` is a runtime/state failure, and `2` is bad usage.
- `--json` may appear anywhere. JSON success and failure responses are emitted
  as one object on stdout and always include `ok`.
- Every mutating phase command requires project, feature, run, and canonical
  repository identity. Identity is never inferred from cwd or conversation.
- `.hermoso` is clone-local and excluded through `.git/info/exclude`.

Use `hermoso help` for the authoritative command list.

## Documentation

- [Setup, TUI/CLI workflow, and recovery](docs/usage.md)
- [Architecture and identity boundaries](docs/architecture.md)
- [Verification contracts, reports, remediation, and publication](docs/verification-contracts.md)
- [Available and planned functionality](docs/status.md)
- [Historical archive](docs/archive/README.md)
