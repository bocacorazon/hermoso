---
name: hermoso-construction
description: "Use when an exact Hermoso design is approved. Author a work-graph, create and bind Kanban work safely, and drive worker and integration cards to truthful blocked or completed outcomes."
version: 1.1.1
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

Never infer identity from cwd, conversation, branch names, or card placement.
Pass explicit context (project-id, feature-id, run-id, repository) to every
`hermoso` command — the binary validates context against persisted state and
rejects mismatches. Proceed only when status proves that the complete
design-package revision/hash is approved. The package includes a visible design
and hidden verification contract/assets. Stop and block rather than dispatching
from conversational memory.

### Constitution Check

Before authoring the work graph or dispatching any work, read the project
constitution:

1. Read `docs/constitution.md` in the target project.
2. If `docs/constitution.md` does not exist, block and report that the
   constitution is missing — the design phase should have enforced this, but
   re-check to be safe.
3. If the constitution exists, read each principle and check the work graph
   against them. Each work item must not violate any constitution principle.
4. Check the approved design's `decisions` array for escape-hatch overrides.
   If a violation is covered by an override entry (the decision names the
   principle being overridden and the rationale explains why), proceed.
5. If a violation is detected and no escape-hatch override exists in the
   approved design, block dispatch. The block reason should name the
   specific principle violated and the work item that violates it.

This is the last gate before workers write code. Constitution violations
caught here prevent costly rework.

## Construction Plan

Before authoring the work graph, produce a brief construction plan at
`docs/features/[slug]/construction-plan.md`. This makes the decomposition
intentional and reviewable.

The plan should include:

1. **Decomposition strategy** — how the approved design is broken into work
   items and why (by layer, by feature, by dependency order).
2. **Dependency analysis** — which items depend on which, and why. Identify
   the critical path.
3. **Parallelization** — which items can run in parallel and which must be
   sequential.
4. **Integration points** — where fan-in is needed and why.
5. **Risk areas** — items that are uncertain, complex, or likely to need
   iteration.

For small features (1-2 work items), the plan can be 2-3 sentences. For
complex features, write a full paragraph per section.

Present the plan to the user and confirm before proceeding to graph authoring.

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

Item prompts must warn workers explicitly to work ONLY inside their assigned
worktree and never write into the main repository checkout — local-model
workers ignore this often enough that it must be stated in every card.

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
2. pass explicit context (project-id, feature-id, run-id, repository) from
   the card to every `hermoso` command; the binary validates against
   persisted state;
3. work only in the exact absolute `$HERMES_KANBAN_WORKSPACE`;
4. inspect parent handoffs before changing code;
5. follow TDD for behavior changes;
6. diagnose failures systematically;
7. run the card's validation commands;
8. commit work when the workspace is a managed worktree;
9. leave a structured handoff with changed files, commit, tests, decisions, and
   remaining risks;
10. pass explicit context to every command, then block or complete truthfully.

Do not edit `.hermoso` files from a worker worktree. Do not use
`delegate_task` instead of Kanban for durable graph work.

## Verify and Salvage Worker Output

A worker's self-reported status is a CLAIM, not evidence. Observed failure
modes (local-model workers, seen repeatedly across features):

1. **Silent death:** if the profile's model endpoint is down, the worker dies
   in ~20 seconds after one heartbeat. The task stays zombied as `running` —
   without a kanban daemon, `max-runtime` is never enforced and nothing reaps
   it. Before dispatching, confirm the endpoint answers (e.g.
   `curl -fsS <base_url>/models`) and restart the serving instance if needed.
2. **Wrong-directory writes:** the worker edits the main repo checkout (or a
   third location) instead of its worktree, then reports success.
3. **Uncommitted / stub output:** the worker writes to the correct worktree
   but never commits, ships stub tests (`pass` bodies), wrong session/API
   shapes, or scripts missing environment seams — and marks the task done.

Orchestrator duties:

- Set up a watcher (poll `hermes kanban show <task>` until the task leaves
  running) — dispatch gives no completion notification.
- On completion, check ALL of: which directories changed (`git status` in the
  kanban worktree, the managed item worktree, and the main checkout), what was
  committed, and run the card's validation commands yourself.
- Salvage pattern when output is partial: fix the defects yourself, commit on
  the worker's branch with an honest message naming what was salvaged vs
  orchestrator-fixed, merge that branch into the managed worktree
  (`git merge --no-edit wt/<task-id>` from the managed worktree), then
  `work complete` with truthful evidence and a `kanban comment` recording the
  salvage.
- Check worker placement early (a few minutes after dispatch), not only at the
  end — catching wrong-directory writes early saves the whole run.
- The kanban dispatcher may spawn workers in its own worktrees
  (`.worktrees/t_<id>`) rather than the Hermoso-prepared item worktrees; the
  worker branch must reach the managed worktree before
  `construction integrate`.

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

## Post-construction ops (verification run and release)

- `hermoso verification run` on emulator-backed contracts or terraform init
  with provider downloads takes many minutes — run it in the background with
  notify; foreground terminal caps are too short.
- After the run, resolve any pending `surface_resolutions` via
  `hermoso verification resolve` (`model_node_id` may be empty when the spine
  has no file-level nodes; mark resolved/missing with a reasoned summary).
- **`hermoso release` merges the feature branch into whatever branch the ROOT
  checkout currently has checked out.** Check out `main` in the root repo
  BEFORE running release, or the merge lands on the wrong branch.
- Release does NOT push — pushing to origin (and any deploy pipeline it
  triggers) is a separate, explicit user decision.

## Common Pitfalls

1. Dispatching before exact approval.
2. Reimplementing the compiler in prompt logic.
3. Creating all cards as ready and linking later.
4. Inventing task IDs or failing to bind returned IDs.
5. Completing while a child or integration card is blocked.
6. Copying skills or profile files into the target repository.
7. Claiming internal repository APIs are current CLI commands.
8. Trusting a worker's "done" without verifying placement, commits, and tests.
9. Dispatching without confirming the profile's model endpoint is up.
10. Expecting max-runtime enforcement without a kanban daemon running.
11. Running `hermoso release` while the root checkout sits on a feature branch.
12. Running long verification contracts in the foreground.

## Verification Checklist

- [ ] Exact current design approval was confirmed by CLI state.
- [ ] Profile model endpoint confirmed up before dispatch.
- [ ] Work graph passed live validation.
- [ ] Graph is minimal and acyclic.
- [ ] Only compiler-returned ready cards were created.
- [ ] Every created card was bound exactly once.
- [ ] Workers used prepared worktrees and required skills.
- [ ] Each worker's output verified (placement, commits, tests) before work complete.
- [ ] Integration exists only for real fan-in.
- [ ] Blocked and completed outcomes match evidence.
- [ ] Root checkout on main before release.
- [ ] Construction result reached `awaiting_verification`.
