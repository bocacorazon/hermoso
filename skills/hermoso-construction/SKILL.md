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

Before authoring the work graph or dispatching any work, resolve the project
constitution:

1. Run `hermoso context constitution <repository>`. It resolves through the
   documented order — `.hermoso/project.json` `constitution_path` override,
   then `docs/constitution.md`, then `.specify/memory/constitution.md`
   (Spec Kit convention) — and reports the resolved path and its source.
   Do NOT assume `docs/constitution.md` is the only location, and do NOT
   author a second constitution to satisfy a missing path.
2. If the command fails, block and report that the constitution is missing
   — the design phase should have enforced this, but re-check to be safe.
3. Read the resolved constitution file, read each principle, and check the
   work graph against them. Each work item must not violate any
   constitution principle.
4. Check the approved design's `decisions` array for escape-hatch overrides.
   If a violation is covered by an override entry (the decision names the
   principle being overridden and the rationale explains why), proceed.
5. If a violation is detected and no escape-hatch override exists in the
   approved design, block dispatch. The block reason should name the
   specific principle violated and the work item that violates it.

This is the last gate before workers write code. Constitution violations
caught here prevent costly rework.

### Spine Constraint Check

After the constitution check, if `docs/spine/index.md` exists in the target
project, read every active `kind: constraint` entry and check the work graph
against them with the same rules as constitution principles: violations
without an escape-hatch override in the approved design's `decisions` array
block dispatch, and the block reason names the spine entry id and the
violating work item. Overrides naming the entry id, recorded and approved at
design time, are honored here. No spine = no-op.

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

**Do not present the plan and wait for confirmation when the user has already
said "go" or "ready."** The design approval was the decision gate. Once the
user says to proceed to construction, push through the plan authoring, work
graph creation, validation, and dispatch without stopping at each sub-step.
Only stop and ask when a genuine ambiguity blocks the next tool call —
profile missing, model endpoint down, constitution violation unrecoverable.
If you have enough context to make the next move, make it.

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

## Implementation Path: Dispatch, Not Direct

**Local-model dispatch via kanban workers is the default construction path.**
The user runs a local fleet specifically to handle construction work —
implementing work items directly (orchestrator-direct) instead of dispatching
workers to that fleet wastes the infrastructure. Only implement directly when:
the endpoint is confirmed down and cannot be restarted, or the change is
self-evidently trivial (one file, one function, one test). When in doubt,
dispatch.

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
13. **Orchestrator-direct implementation in the wrong checkout.** When
    bypassing kanban dispatch and implementing work items directly, you must
    commit to the feature worktree (or merge your main-checkout commits into
    it) before calling `construction integrate`. The integrate command records
    `integrated_feature_commit` from the feature worktree's current HEAD. If
    the work lives only in the main checkout, the feature worktree stays at
    the pre-construction commit, verification runs against that stale commit,
    and every rubric judgment fails with "missing" evidence. After
    implementing in the main checkout: merge main into the feature worktree
    (`git -C <feature-worktree> merge <main-branch> --no-edit`), then call
    `construction integrate` so it records the correct commit.
14. **Work-graph schema: `worker.skills` entries are objects, not strings**
    (`{"name": "<skill>"}` — SkillBinding). The top level also rejects unknown
    fields (e.g. `decode work-graph: unknown field "plan_summary"`) and
    requires `revision`. Generate the graph with a builder script; when
    `validate`/`graph put` names an unknown field or a shape error, re-read
    `hermoso schema work-graph --json` (`$defs` list the exact required
    fields) instead of guessing.
15. **`construction ready` emits cards dependency-ordered, not all at once.**
    Only items whose parents are complete come back ready. On the first call
    expect just the root lane(s); after each `work complete`, re-run
    `construction ready` to receive the next wave. This polling loop is what
    replaces "create all cards and link later".
16. **Pre-profile runs: `init --profile` does NOT retro-bind an existing
    project** — it returns the existing project unchanged, so
    `construction prepare` still fails with `no profile configured for
    project or run`. Fix: surgically add
    `"profile_path": "<abs path to profile yaml>"` to
    `.hermoso/project.json` (a narrow, deliberate exception to "never edit
    `.hermoso/**`" — touch only that one key) and confirm via
    `hermoso status --json` (the project payload echoes `profile_path`).
    Full worked dispatch sequence: `references/construction-dispatch-playbook.md`
    in the `hermoso` skill.
17. **`hermes kanban tail <id>` streams live and blocks a foreground call
    forever.** For a quick post-dispatch sanity check use
    `hermes kanban runs <id> --json` (status, worker_pid, outcome) plus
    `ps -p <pid> -o args` to confirm the worker process and its model flags.
18. **Proving the worker runs on the local LLM.** `ps -p <pid>` must show
    `-m <model> --provider <registered-provider>`; then hit the local
    llama-server `/metrics` and watch `llamacpp:prompt_tokens_total` climb —
    positive proof inference traffic flows to the local endpoint. The card's
    `--model`/`--provider` must reference a `custom_providers` entry in
    `~/.hermes/config.yaml` (register one pointing at the local instance if
    missing), and `--skill` values must be real skill names
    (`test-driven-development`, `systematic-debugging` — a non-skill like
    `python` kills the worker at init).
19. **Local-LLM endpoint death mid-run zombifies workers.** If the model
    server restarts (systemd auto-restart, kernel OOM kill — llama-server
    under concurrent workers exits 137 via systemd-oomd), every in-flight
    worker dies mid-turn but kanban keeps showing `running` (stale claims).
    Signature: `hermes kanban runs <id> --json` says running, but
    `ps -p <pid>` is empty and the task log's timestamp stops at the
    restart; `/metrics` token counters reset to 0 even though `/health`
    says ok (systemd already restarted it). Confirm root cause in
    `journalctl --user -u <llm-service>` (look for `status=137` /
    `oom-kill` / "Killed process ... llama-server"). Recovery:
    `hermes kanban reclaim <task-id>` for each, then
    `hermes kanban dispatch --max N` — workers' uncommitted worktree
    changes survive, and resumed workers continue in the same worktrees.
    Prevention: before dispatching 2+ concurrent workers, check `free -h`
    headroom and llama-server RSS, and size the server's `CTX_SIZE` to the
    actual workload (construction cards need ~32-64K, not 131K — KV cache
    at full context is what OOMs a 29 GB box). Full incident walk-through:
    `references/llm-endpoint-outage-recovery.md`.
20. **A dirty managed worktree blocks `work start`.** Scratch dirs workers
    create (`.venv-lane/`, etc.) make `hermoso work start` fail with
    `managed worktree is dirty: <path>`. Fix in the affected item worktree:
    append the scratch dir to `.gitignore`, `git add .gitignore`, commit.
    Do this proactively for every lane before state transitions.
21. **Child lanes can branch off the base despite synchronized parents.**
    `work start` claims to synchronize parents, but a child lane's worktree
    may still sit on the pre-parent commit — the worker's "green suite"
    then ran against STALE code and its files collide at integration.
    Always check `git log --oneline -3` in each lane worktree contains the
    parent's commit before accepting the lane. Remedy:
    `git -C <lane-worktree> merge <parent-sha> -m "..."`, re-run the full
    suite in that worktree, then continue verification.
22. **Verification contracts pin exact CLI semantics — fix the script, not
    the contract.** When a judgment command (e.g.
    `script --export X --db Y --verify` expecting exit 0) fails because the
    worker implemented different flag semantics (`--verify` meaning
    verify-only, which mismatches on a fresh db), the approved contract is
    the pinned artifact: change the script so the contract's exact command
    passes, and move the other semantics to a separate flag
    (`--verify-only`). Make such scripts idempotent (skip already-present
    rows) so re-running the judgment command is safe.
23. **`construction ready` returns `cards: null` once every pending card is
    bound.** A bound-but-never-spawned card (lost race, OOM window) does
    NOT reappear in ready output — `Ready` skips bound items. Just
    `hermes kanban dispatch --max N` directly; the binding is intact.
24. **Exit codes lie through pipes.** `cmd | tail -2; echo "exit=$?"`
    reports tail's exit code, not the command's — a red judgment can look
    green. When verifying judgments, run the command bare (output is
    auto-truncated and saved) or redirect to a file, then echo `$?`.
25. **Verification contracts may pin an absolute interpreter path** (e.g.
    `/home/marcos/.venvs/<name>/bin/python` in every judgment command).
    That venv must exist with the project's full dependency set BEFORE
    `verification run` or every judgment fails. Rebuild it proactively
    during construction:
    `uv venv <path> && uv pip install --python <path>/bin/python -r requirements.txt pytest`,
    and verify importability of the key deps first.
26. **Worker commits land on `wt/<task-id>` branches in kanban-owned
    worktrees, NOT the Hermoso-prepared item worktrees.** The commits are in
    the shared object database, so cherry-pick them into the managed worktree:
    `cd <hermoso-item-worktree> && git cherry-pick <sha>` (skip if
    already-present — empty cherry-pick means the commit already landed).
    Then `work complete`. The kanban worktree's branch is ephemeral;
    the Hermoso worktree is the canonical lane record.
27. **A `started` work item can NOT be bound to a kanban card** —
    `task bind` rejects it with \"not bindable from started.\"
    This happens when the orchestrator ran `work start` during a manual
    implementation attempt earlier in the session. Recovery: implement
    the item directly in the managed worktree (the state is already
    correct), commit, and `work complete`. Next time, only `work start`
    immediately before dispatch, not during planning.

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
