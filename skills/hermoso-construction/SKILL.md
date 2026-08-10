---
name: hermoso-construction
description: "Use when an exact Hermoso design is approved. Author a work-graph, create and bind Kanban work safely, and drive worker and integration cards to truthful blocked or completed outcomes."
version: 1.0.0
author: Hermoso
license: MIT
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [hermoso, construction, kanban, worktrees, integration]
    related_skills: [hermoso, hermoso-design]
---

# Hermoso Construction Lifecycle

## Overview

Convert an explicitly approved design into a validated work graph and, once
the vertical CLI surface exists, deterministic Kanban cards and task bindings.
Hermoso owns graph/state invariants, Hermes Kanban owns dispatch, and workers
own code in their assigned worktrees.

## Entry Gate

Refresh:

```sh
hermoso status <repository> --json
hermoso context <project-id> <feature-id> <run-id> <repository> --json
```

Echo and compare project, feature, run, canonical repository, and absolute
workspace at entry and every graph/card boundary. Never infer them from cwd,
conversation, branch names, or card placement. Block immediately on mismatch.
Proceed only when status proves that the complete design-package revision/hash
is approved. The package includes a visible design and hidden verification
contract/assets. Stop and block rather than dispatching from conversational
memory.

## Author the Work Graph

Always begin with:

```sh
hermoso schema work-graph --json
```

Write outside `.hermoso/**`, then:

```sh
hermoso validate work-graph <work-graph-path> <project-id> <feature-id> <run-id> <repository> --json
```

Rules:

- one item is a valid graph;
- parents represent actual data or sequencing dependencies;
- independent items have no parent links;
- every item has a real configured profile and ordered skills;
- acceptance criteria derive from the approved design;
- every item cites visible `requirement_ids`, `acceptance_criterion_ids`, and
  `surface_ids`; the graph covers the complete visible design;
- expected changed surfaces help detect overlap but are not ownership locks;
- validation commands are concrete and safe to run in the worktree;
- runtime budgets are proportional;
- add an integration item only when multiple graph leaves need fan-in.

Use `test-driven-development` for production changes and
`systematic-debugging` when a test, merge, build, or integration check fails.

## Compile, Create, Bind

Use the delivered vertical interface:

```sh
hermoso graph put <project-id> <feature-id> <run-id> <repository> <work-graph-path> --json
hermoso construction prepare <project-id> <feature-id> <run-id> <repository> <profile-path> --json
hermoso construction ready <project-id> <feature-id> <run-id> <repository> --json
hermoso task bind <project-id> <feature-id> <run-id> <repository> <work-item-id> <kanban-task-id> --json
hermoso work start <project-id> <feature-id> <run-id> <repository> <work-item-id> --json
hermoso work complete <project-id> <feature-id> <run-id> <repository> <work-item-id> <evidence-id> <summary> <command> --json
hermoso work block <project-id> <feature-id> <run-id> <repository> <work-item-id> <evidence-id> <reason> <command> --json
hermoso resume <project-id> <feature-id> <run-id> <repository> --json
hermoso construction integrate <project-id> <feature-id> <run-id> <repository> [check ...] --json
hermoso result put <project-id> <feature-id> <run-id> <repository> <phase-result-path> --json
```

The compile response must provide, per card: title, body, resolved logical
parents, tenant, priority, prepared absolute worktree path, assigned profile,
ordered forced skills, runtime budget, goal mode, lifecycle commands,
acceptance criteria, and idempotency key.
The card identity, body, and lifecycle commands must repeat the full context;
the tenant is derived from `project_id`, and idempotency covers the full
context. Workers refresh `hermoso context` and compare every field before work,
validation, handoff, block, or completion.

Cards may contain visible requirement/criterion text, planned or existing
surface descriptions, constraints, and the exact model reference. They must
not contain verification-contract paths or hashes, judgment/scenario IDs or
text, hidden fixture/probe paths, or verifier commands. Treat any such field as
a disclosure defect and block before dispatch.

Process cards in topological readiness order:

1. persist and compile through Hermoso;
2. create only cards returned as ready;
3. call `kanban_create`/the equivalent tool with the exact compiled fields,
   including `parents`, `workspace`, `assignee`, repeated skills, runtime,
   tenant, priority, and idempotency key;
4. capture the returned task ID;
5. bind it immediately with the Hermoso CLI;
6. refresh status before creating newly ready cards.

Never create a card without binding it. If creation succeeds but binding fails,
block and report the orphan task ID; do not create a duplicate.

## Worker Lifecycle

Each worker must:

1. call `kanban_show` and verify the card is active;
2. echo the card context and absolute workspace, refresh `hermoso context`,
   compare all fields, and block on mismatch;
3. work only in the exact absolute `$HERMES_KANBAN_WORKSPACE`;
4. inspect parent handoffs before changing code;
5. follow TDD for behavior changes;
6. diagnose failures systematically;
7. run the card's validation commands;
8. commit work when the workspace is a managed worktree;
9. leave a structured handoff with changed files, commit, tests, decisions, and
   remaining risks;
10. refresh and compare context again, then block or complete truthfully.

Do not edit `.hermoso` files from a worker worktree. Do not use
`delegate_task` instead of Kanban for durable graph work.

## Dependency and Integration Lifecycle

Parent completion makes a dependent card eligible; prose saying “wait for X”
does not. A child must consume the actual parent handoff and synchronized
worktree state.

Multiple leaves require integration only when their changes must be merged or
validated together. Integration work should:

- use the prepared Hermoso integration/feature worktree;
- merge exact managed parent branches;
- block on conflicts rather than discarding edits;
- run baseline checks after fan-in;
- record conflict paths or failing commands as evidence.

`construction integrate` drives pairwise managed integration. On conflict it
blocks without resetting files; after the resolution is staged, `resume` and a
retry continue the same merge safely.

## Block and Complete Correctly

Call `kanban_block` when:

- design approval is absent or stale;
- a profile or required skill is unavailable;
- card creation succeeded but Hermoso binding failed;
- a parent handoff is missing;
- a merge conflicts;
- validation fails after root-cause investigation;
- human or environment action is required.

Use `kanban_comment` first for detailed evidence. A block reason should name the
specific decision or action needed.

Call `kanban_complete` only when all acceptance criteria and validations pass.
For code needing human review, follow `kanban-worker` and use a
`review-required:` block. Construction is complete only when every required
work item is done, integration checks pass, no unresolved blockers remain, and
the validated construction `phase-result` is persisted by `hermoso result put`.
The resulting `awaiting_verification` state is handled by
`hermoso-verification`. A first failed attempt may create one remediation round;
dispatch it through the same ready/bind/start/complete lifecycle. Do not create
or accept a third construction round.

## Common Pitfalls

1. Dispatching before exact approval.
2. Reimplementing the compiler in prompt logic.
3. Creating all cards as ready and linking later.
4. Inventing task IDs or failing to bind returned IDs.
5. Completing while a child or integration card is blocked.
6. Copying skills or profile files into the target repository.
7. Claiming internal repository APIs are current CLI commands.

## Verification Checklist

- [ ] Exact current design approval was confirmed by CLI state.
- [ ] Work graph passed live validation.
- [ ] Graph is minimal and acyclic.
- [ ] Only compiler-returned ready cards were created.
- [ ] Every created card was bound exactly once.
- [ ] Workers used prepared worktrees and required skills.
- [ ] Integration exists only for real fan-in.
- [ ] Blocked and completed outcomes match evidence.
- [ ] Construction result reached `awaiting_verification`.
