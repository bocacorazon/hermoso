---
name: hermoso
description: "Use when coordinating a Hermoso feature from spine-grounded design through atomic contract approval, construction, verification/remediation, and Gherkin publication."
version: 1.0.0
author: Hermoso
license: MIT
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [hermoso, tui, orchestration, design, construction, kanban]
    related_skills: [hermoso-design, hermoso-verification-author, hermoso-construction, hermoso-verification]
---

# Hermoso TUI Controller

## Overview

This is the user-facing controller for a Hermoso run. Keep the developer in the
Hermes TUI, use `hermoso` as the authority for contracts and durable lifecycle
state, and use Hermes Kanban only for dispatch and visibility.

Never edit `.hermoso/**` directly. Never infer project, feature, run, repository,
or lifecycle identity from cwd, chat, branches, worktrees, or Kanban. Every
operation starts from an explicit repository and run, refreshed with
`hermoso context <project-id> <feature-id> <run-id> <absolute-repository> --json`.

## When to Use

- Starting, inspecting, or resuming a Hermoso feature.
- Routing an approved design into construction.
- Presenting blockers, approval requests, construction progress, or
  verification reports.

Do not use this skill as a generic project manager or as permission to install
files into a target repository.

## Authority Order

1. `hermoso context <project-id> <feature-id> <run-id> <repository> --json` for canonical identity.
2. `hermoso status <repository> --json` for persisted project/run state.
3. `hermoso schema <kind> --json` for the live contract shape.
4. `hermoso validate <kind> <path> <project-id> <feature-id> <run-id> <repository> --json`.
5. Hermes Kanban for external task status and worker handoffs.
6. Conversation context only for objectives and human decisions not yet
   persisted.

If these disagree, stop and report the disagreement. Do not repair state by
editing JSON.

## Available CLI Calls

These commands exist now:

```sh
hermoso init /absolute/target/repository --json
hermoso start <feature-id> /absolute/target/repository --json
hermoso status /absolute/target/repository --json
hermoso context <project-id> <feature-id> <run-id> /absolute/target/repository --json
hermoso schema feature-design --json
hermoso schema feature-verification-contract --json
hermoso schema work-graph --json
hermoso schema phase-result --json
hermoso validate feature-design <path> <project-id> <feature-id> <run-id> /absolute/target/repository --json
hermoso validate work-graph <path> <project-id> <feature-id> <run-id> /absolute/target/repository --json
hermoso validate phase-result <path> <project-id> <feature-id> <run-id> /absolute/target/repository --json
hermoso design put <project-id> <feature-id> <run-id> /absolute/target/repository <path> --json
hermoso verification put <project-id> <feature-id> <run-id> /absolute/target/repository <path> --json
hermoso approve design <project-id> <feature-id> <run-id> /absolute/target/repository <package-revision> <package-hash> <actor> [comment] --json
hermoso graph put <project-id> <feature-id> <run-id> /absolute/target/repository <path> --json
hermoso construction prepare <project-id> <feature-id> <run-id> /absolute/target/repository <profile-path> --json
hermoso construction ready <project-id> <feature-id> <run-id> /absolute/target/repository --json
hermoso construction integrate <project-id> <feature-id> <run-id> /absolute/target/repository [check ...] --json
hermoso task bind <project-id> <feature-id> <run-id> /absolute/target/repository <work-item-id> <task-id> --json
hermoso work start <project-id> <feature-id> <run-id> /absolute/target/repository <work-item-id> --json
hermoso work complete <project-id> <feature-id> <run-id> /absolute/target/repository <work-item-id> <evidence-id> <summary> <command> --json
hermoso work block <project-id> <feature-id> <run-id> /absolute/target/repository <work-item-id> <evidence-id> <reason> <command> --json
hermoso result put <project-id> <feature-id> <run-id> /absolute/target/repository <path> --json
hermoso verification run <project-id> <feature-id> <run-id> /absolute/target/repository --json
hermoso resume <project-id> <feature-id> <run-id> /absolute/target/repository --json
```

`--json` is the skill-facing interface. Check `ok`; never scrape text output.

## Controller Flow

1. Run `hermoso status <repo> --json`.
2. If uninitialized, explain the clone-local state boundary, then run the
   available `hermoso init <repo> --json`.
3. If no matching run exists, run the available
   `hermoso start <feature-id> <repo> --json`.
4. Resolve `hermoso context`, echo all four identities, and compare them with
   status. Block on any mismatch.
5. Invoke `hermoso-design` with the complete context, absolute workspace,
   objective, and
   `profiles/default.yaml` bindings.
6. Invoke `hermoso-verification-author` to create and ingest the hidden contract
   and sealed assets before any work graph is authored.
7. Present the visible design plus verification coverage/modalities/exclusions
   and exact package revision/hash.
8. Require an explicit user approval of that exact package. Silence,
   earlier approval, approval of a summary, or “continue” before review is not
   approval.
9. If changes are requested, revise and validate again. The previous approval
   is stale.
10. Invoke `hermoso-construction` only after exact-package approval is durably
   recorded by `hermoso approve design`.
11. At `awaiting_verification`, invoke `hermoso-verification`. Route its one
   automatic remediation round through construction, or surface a second
   failure/block. A pass publishes approved Gherkin and reaches
   `awaiting_release`.
12. Refresh `hermoso status --json` after every persisted transition and before
   declaring completion. At every phase boundary, refresh `hermoso context`,
   echo the full tuple and absolute workspace, compare them, and block on any
   mismatch.

## Explicit Approval Gate

Persist and approve only through the CLI:

```sh
hermoso design put <project-id> <feature-id> <run-id> <repository> <feature-design-path> --json
hermoso verification put <project-id> <feature-id> <run-id> <repository> <verification-contract-path> --json
hermoso approve design <project-id> <feature-id> <run-id> <repository> <package-revision> <package-hash> <actor> [comment] --json
```

The implementation rejects approval until both package parts cross-validate,
and rejects any revision/hash or context mismatch.

## Right-Sized Design

- One construction item is valid and preferred for a cohesive small change.
- Add an optional spike only for a real uncertainty that cannot be resolved by
  reading code or documentation.
- Use a DAG only when work is genuinely parallel or one item truly consumes
  another's output.
- Do not create planner, reviewer, integration, or ceremony cards by default.
- Add integration work only when multiple leaves must be combined or validated
  together.

## Blocking and Completion

When spawned on a Kanban card, follow `kanban-worker`:

- use `kanban_comment` for durable context;
- call `kanban_block` for unresolved human, environment, approval, merge, or
  validation blockers;
- call `kanban_complete` only when the card's acceptance criteria are actually
  satisfied;
- never complete a parent controller card while required child cards are
  blocked, failed, running, or unbound.

For a blocked Hermoso phase, author a valid `phase-result` with
`status: "blocked"` and at least one concrete `unresolved_blockers` entry, then
validate it, then persist it with `hermoso result put` using the full context.

## Common Pitfalls

1. Editing `.hermoso/runs/*/run.json` to advance a phase.
2. Treating `hermoso validate` as persistence or approval.
3. Dispatching after conversational approval that was not recorded.
4. Creating a multi-card DAG for a one-item change.
5. Inventing profile names instead of using the selected profile file and
   `hermes profile list`.
6. Copying these skills into the target repository.

## Verification Checklist

- [ ] Status was read through `hermoso status --json`.
- [ ] Contracts came from the live `schema` command.
- [ ] Authored contracts passed the live `validate` command.
- [ ] No `.hermoso` file was edited directly.
- [ ] Exact complete design-package approval is durable before dispatch.
- [ ] Hidden verifier content was not sent to build agents.
- [ ] Every verification attempt produced a persisted report.
- [ ] Kanban blockers and completions reflect reality.
- [ ] Delivered commands were invoked with the complete canonical context.
