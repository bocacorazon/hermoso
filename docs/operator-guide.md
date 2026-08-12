# Hermoso Operator Guide: How to Use the Full Process

This guide explains, for each phase of the Hermoso lifecycle, what you (the
human operator) actually do: which tool you interact with, what you type,
what skill fires, what output you get, and what you do with it. It assumes
you have Hermes (the TUI/agent) running and the Hermoso CLI installed.

---

## Tools You Interact With

You switch between two tools throughout the process:

- **Hermes** — your AI agent (TUI, CLI, or messaging channel). You talk to
  it in natural language. It loads Hermoso skills, reasons about your
  feature, and writes contract files.
- **Hermoso CLI** — the deterministic control plane (`hermoso` command). It
  validates, persists, and transitions state. You run it directly or Hermes
  runs it for you via its terminal tool.

In practice, you mostly talk to Hermes. Hermes runs `hermoso` commands
under the hood and reports results back to you. You can also run `hermoso`
commands yourself in a terminal.

---

## Phase 0: Setup (one-time per repository)

**What you do:** In your repository, run:

```
hermoso init . [--profile <path-to-profile.yaml>]
```

**What happens:** Hermoso creates `.hermoso/` with `project.json` (project
ID, repository identity, kanban tenant). It adds `.hermoso/` to
`.git/info/exclude` so state is never committed.

**Output:** "initialized Hermoso project project-xxxx in /path/to/repo"

**What you do with it:** Note the project ID. You'll pass it to every
subsequent command. (Or just let Hermes handle it — it reads status to get
the ID.)

**Profile (optional):** If you pass `--profile`, the project records the
path. This profile controls which model/skills each phase uses. Without it,
phases use the global default Hermes profile. See
[profile-resolution-mechanics.md](./profile-resolution-mechanics.md).

---

## Phase 1: Start a Feature Run

**What you do:** Tell Hermes:

> "Start a new Hermoso feature run for [feature description] in [repo path]"

Or run directly:

```
hermoso start <feature-id> . [--profile <path>]
```

The feature ID is a short slug you choose (e.g. `add-rate-limiting`).

**What happens:** Hermoso creates a run file at `.hermoso/runs/<run-id>.json`
with phase=design, status=pending.

**Output:** "started run run-xxxx for feature add-rate-limiting"

**What you do with it:** Nothing yet — the next step is design. Hermes now
has the context tuple (project-id, feature-id, run-id, repo) it needs for
all subsequent commands.

---

## Phase 2: Design

This phase has three sub-steps. You interact with Hermes throughout; Hermes
runs the Hermoso CLI commands.

### Step 2a: Author the Feature Design

**What you do:** Tell Hermes:

> "Design a feature for [objective]. Constraints: [list]. Here's the repo
> context."

**What Hermes does:**
1. Loads the `hermoso-design` skill.
2. Runs `hermoso status`, `hermoso context`, `hermoso schema feature-design`
   to get the live schema and current state.
3. Runs `hermoso model build` (if model is stale or missing) to index your
   codebase into a SCIP-based knowledge graph.
4. Runs `hermoso model query orientation` to understand what the codebase
   does. Runs `hermoso model query task "<your objective>"` to find relevant
   files.
5. Reasons about complexity (small/standard/complex) and drafts a
   `feature-design.json` with: objective, requirements (each with a stable
   ID like `req-1`), acceptance criteria (each linked to requirement IDs),
   business vocabulary, interaction surfaces (referencing model node IDs),
   decisions, and tradeoffs.
6. Writes the file outside `.hermoso/` (e.g. in the repo root or a
   `designs/` directory).
7. Runs `hermoso validate feature-design <path> <context> --json` to check
   it against the live schema.

**Output:** A validated `feature-design.json` file on disk, plus Hermes
presents a summary: objective, acceptance criteria, complexity assessment,
proposed work items, and any open questions.

**What you do with it:** Read the summary. Answer any open questions Hermes
asks. If you want changes, tell Hermes and it will increment the revision
and re-validate. When you're satisfied, say "proceed" or "persist it."

**What Hermes does next:**
1. Runs `hermoso design put <context> <path> --json`
2. Hermoso persists the design into the run state, computes a content hash,
   sets status=in_progress.

**Output:** "persisted feature design revision 1 sha256:abc..."

### Step 2b: Author the Verification Contract

**What you do:** Nothing — this happens automatically. After `design put`,
Hermes loads the `hermoso-verification-author` skill.

**What Hermes does:**
1. Runs `hermoso schema feature-verification-contract` to get the live
   schema.
2. Traces every requirement and acceptance criterion to a verification
   judgment or an explicit exclusion.
3. For behavioral requirements, writes Gherkin `.feature` files with
   scenarios tagged `@requirement:req-1`, `@criterion:crit-1`,
   `@surface:surf-1`, etc.
4. For non-behavioral checks (performance, security, structure), writes
   deterministic command-based or rubric-based judgments.
5. Hashes every artifact (Gherkin files, fixtures) and assembles a
   `feature-verification-contract.json` with: judgments, artifacts,
   publication paths, oracle types.
6. Runs `hermoso validate feature-verification-contract <path> <context>
   --json`.
7. Runs `hermoso verification put <context> <path> --json`. Hermoso reads
   and seals all artifacts, computes the verification hash, artifact root
   hash, and the atomic package hash.

**Key concept — operational non-disclosure:** The verification contract,
Gherkin scenarios, and verifier assets are HIDDEN from construction workers.
Workers see the feature design only. This prevents workers from
gaming the tests. Hermoso enforces this through worktree separation —
verification assets live in `.hermoso/`, never in construction worktrees.

**Output:** Hermes presents: package revision + hash, verification coverage
(which requirements have judgments, which are excluded and why), modalities
(Gherkin, command, rubric), publication paths, and oracle types.

**What you do with it:** Review the coverage. Make sure every requirement
is covered by a judgment or explicitly excluded with rationale. If gaps,
ask Hermes to add coverage.

### Step 2c: Approve the Design Package

**What you do:** Tell Hermes:

> "Approve the design package."

Or Hermes will ask you for explicit approval.

**What Hermes does:**
1. Runs `hermoso approve design <context> <revision> <package-hash>
   <your-name> [comment] --json`
2. Hermoso verifies the revision and hash match exactly, records the
   approval, transitions phase=construction, status=in_progress.

**Output:** "approved exact design revision"

**What you do with it:** Construction can now begin. The design is locked.
Any change to the design, verification contract, artifacts, or model
invalidates the approval and requires a new package revision.

---

## Phase 3: Construction

This is the longest phase. You interact with Hermes to orchestrate, and
Hermes dispatches work to worker agents via Hermes Kanban.

### Step 3a: Author the Work Graph

**What you do:** Nothing explicit — Hermes loads the `hermoso-construction`
skill after approval.

**What Hermes does:**
1. Runs `hermoso schema work-graph --json` to get the live schema.
2. Based on the approved feature design, drafts a `work-graph.json` — a DAG
   of work items. Each work item has: title, body (instructions), parent
   dependencies (only when a child can't start without the parent's output),
   profile binding, ordered skill list (typically: `kanban-worker`,
   `hermoso-construction`, `test-driven-development`), acceptance criteria
   (from the approved design), expected changed surfaces, validation
   commands, and runtime budget.
3. Runs `hermoso validate work-graph <path> <context> --json`.
4. Runs `hermoso graph put <context> <path> --json`. Hermoso persists the
   graph as a construction round (round 1).

**Output:** "persisted work graph revision 1 sha256:abc..."

**What you do with it:** Review the graph Hermes presents. Check that:
- Dependencies are real (not ceremonial).
- Each item is cohesive (not over-split).
- Skills and profiles are correct.

### Step 3b: Prepare Construction (Create Worktrees + Cards)

**What you do:** Tell Hermes "prepare construction" or it does this
automatically after graph put.

**What Hermes does:**
1. Runs `hermoso construction prepare <context> [profile-path] --json`.
2. Hermoso's dispatch compiler converts the work graph into Kanban card
   specifications. For each work item:
   - Creates a git worktree at `.hermoso/worktrees/<workspace-id>/`,
     branched from the feature branch (or parent work item's branch).
   - Compiles a Card with: title, body, parents, tenant, priority,
     workspace path, assigned profile, forced skills, lifecycle commands
     (the exact `hermoso` CLI commands for prepare/execute/validate),
     acceptance criteria, base model reference, and idempotency key.
3. Returns the plan with all cards.

**Output:** "prepared N construction cards" — each card has a work item ID
and an absolute worktree path.

### Step 3c: Dispatch Ready Cards

**What you do:** Tell Hermes "dispatch ready cards" or it does this
automatically.

**What Hermes does:**
1. Runs `hermoso construction ready <context> --json` — returns cards whose
   parent work items are all completed (or cards with no parents).
2. For each ready card:
   - Creates a Kanban task via `kanban_create` with the exact compiled
     fields (title, body, workspace, assignee, skills, runtime, tenant,
     priority, idempotency key).
   - Captures the returned task ID.
   - Binds it immediately: `hermoso task bind <context> <work-item-id>
     <kanban-task-id> --json`.
3. Hermes Kanban dispatches each task to a worker agent (a separate Hermes
   session running the assigned profile + skills).

**Key rule:** Every created card must be bound. If creation succeeds but
binding fails, Hermes blocks and reports the orphan task ID.

**Output:** "N card(s) dispatched" — each with a Kanban task ID.

### Step 3d: Workers Execute

**What you do:** Wait. Workers run autonomously in separate Hermes sessions.
You can monitor progress via the Kanban board (`hermes kanban list`) or
wait for Hermes to report completions.

**What each worker does:**
1. Calls `kanban_show` to read its assigned card.
2. Verifies the card is active (not blocked/archived).
3. `cd`s into its assigned worktree
   (`$HERMES_KANBAN_WORKSPACE`).
4. Reads parent handoffs (comments from parent work items, if any).
5. Follows TDD: writes tests first, then implementation.
6. Runs the card's validation commands.
7. Commits work in the worktree.
8. Calls `kanban_complete(summary=..., metadata=...)` with changed files,
   test counts, decisions, and remaining risks. Or calls
   `kanban_block(reason="review-required: ...")` if human review is needed.

**If a worker blocks:** The Kanban board shows the blocked task with the
reason. You (or Hermes) address the blocker and unblock:
`hermes kanban unblock <task-id>`. The worker respawns with the comment
thread.

**If a worker times out or crashes:** Hermes Kanban reclaims the task. The
next dispatch spawns a fresh worker that can see prior run outcomes and
avoid repeating the same path.

### Step 3e: Dispatch Next Wave

**What you do:** When workers complete, tell Hermes "check ready cards" or
Hermes does this automatically on completion notifications.

**What Hermes does:**
1. Runs `hermoso construction ready <context> --json` again.
2. Newly eligible cards (whose parents just completed) are returned.
3. Dispatches them the same way as Step 3c.

This repeats until all work items are completed or blocked.

### Step 3f: Record Work Outcomes

**What Hermes does (for each work item):**
1. Runs `hermoso work start <context> <work-item-id> --json` (when the
   worker begins — marks the item as started).
2. When the worker completes: `hermoso work complete <context>
   <work-item-id> <evidence-id> "<summary>" "<command>" --json` — records
   evidence (command run, summary, timestamp), marks the item completed.
3. If blocked: `hermoso work block <context> <work-item-id> <evidence-id>
   "<reason>" "<command>" --json` — marks blocked with reason.

**Output:** Each command updates the run state. You can check progress
anytime with `hermoso status <repo> --json`.

### Step 3g: Integrate

**What you do:** Tell Hermes "integrate construction" when all work items
are done.

**What Hermes does:**
1. Runs `hermoso construction integrate <context> --json`.
2. Hermoso merges each work-item branch back into the feature branch in
   topological order.
3. Records integration commit, integrated leaves, and timestamp.
4. Transitions phase=verification, status=awaiting_verification.

**If merge conflicts:** Hermoso blocks without discarding edits. You
resolve the conflict in the worktree, stage the resolution, then run
`hermoso resume <context> --json` to retry the merge.

**Output:** "construction branches integrated" — feature branch now has
all changes.

### Step 3h: Persist Construction Result

**What Hermes does:**
1. Writes a `phase-result.json` with: phase, status (completed/blocked),
   changed files, test results, decisions, remaining risks.
2. Runs `hermoso result put <context> <path> --json`.

**Output:** "construction result persisted"

---

## Phase 4: Verification

Hermes loads the `hermoso-verification` skill. You mostly wait and then
make routing decisions based on results.

### Step 4a: Run Verification

**What you do:** Tell Hermes "run verification" or it does this
automatically after construction completes.

**What Hermes does:**
1. Runs `hermoso verification run <context> --json`.
2. Hermoso:
   - Verifies the integrated candidate commit and package hashes are
     unchanged.
   - Creates an isolated verification worktree (separate from construction
     worktrees).
   - Materializes the hash-locked Gherkin and verifier assets.
   - Builds a candidate model snapshot.
   - Executes the approved verification commands (Gherkin runner, etc.).
   - Records: command output, exit code, duration, stdout/stderr, before/
     after fingerprints.
   - Persists a verification report.

**Output:** "verification attempt 1: pass" or "verification attempt 1:
fail" or "verification attempt 1: judgment_pending"

**The verdict depends on the oracle type:**
- **Gherkin oracle:** The BDD runner's exit code IS the verdict. Pass =
  all scenarios pass. Fail = one or more scenarios fail. No human judgment
  needed — Hermoso evaluates this mechanically.
- **Rubric oracle:** Hermoso records evidence but does NOT evaluate
  quality. Status = `judgment_pending`, transitions to
  `awaiting_judgment`.

### Step 4b: Apply Judgments (if judgment_pending)

**What you do:** Hermes handles this, but you may need to provide input on
rubric judgments.

**What Hermes does:**
1. Reads the Phase 1 report. Identifies pending outcomes.
2. For **pending surface resolutions** (planned surfaces Go couldn't
   deterministically match to model nodes):
   - Runs `hermoso model query <project> <repo> evidence <surface-id>
     --json` to find the matching code node.
   - Determines whether the candidate implemented the surface.
   - Writes a `resolutions.json` with status: "resolved" or "missing" for
     each surface.
   - Runs `hermoso verification resolve <context> <resolutions.json>
     --json`.
3. For **pending rubric judgments:**
   - Applies the rubric criteria defined in the verification contract.
   - Writes a `judgments.json` with: judgment_id, status (pass/fail),
     reasoning, criteria references.
   - Runs `hermoso verification judge <context> <judgments.json> --json`.

**Output:** "verification judged: verdict=pass" or "verification judged:
verdict=fail"

### Step 4c: Routing (what happens based on the verdict)

**If PASS:**
- Hermoso automatically publishes the approved Gherkin `.feature` files
  into the feature branch (a distinct commit after the candidate commit).
- Refreshes the repository model at the publication commit.
- Annotates the model with published scenario links.
- Transitions to status=awaiting_release.
- **Output:** "verification resolved: verdict=pass" — Gherkin published,
  feature branch has verified scenarios.

**If FAIL (first attempt):**
- Transitions to status=awaiting_remediation.
- Hermes reads the failed report (outcomes, evidence, summaries).
- Hermes authors a `remediation-spec.json` with targeted RemediationNeeds
  (expected vs. actual for each failure) and a new WorkGraph for the fix.
- Runs `hermoso verification remediate <context> <spec.json> --json`.
- Hermoso creates construction round 2, transitions to
  phase=construction, status=in_progress.
- **You're back in Phase 3** — run `construction prepare`, dispatch ready
  cards, workers fix the issues, integrate, then re-verify.
- **Output:** "remediation round 2 persisted" — go back to Step 3b.

**If FAIL (second attempt):**
- The run is blocked. No third construction round is allowed.
- **Output:** "verification blocked" — you need to decide: redesign
  (return to Phase 2 with a new design package) or cancel the run.

**If BLOCKED (infrastructure issue):**
- Fix the external condition (e.g. missing dependency, network issue).
- Run `hermoso resume <context> --json` then `hermoso verification run
  <context> --json` to retry.
- This does NOT consume a semantic attempt — the incident is archived.

---

## Phase 5: Release

**What you do:** The feature branch now contains verified code + published
Gherkin scenarios. You merge it to your main branch (or open a PR) using
standard git:

```
git checkout main
git merge feature/<feature-id>
```

Or push and open a PR for review.

Hermoso does not perform the merge to main — it verifies and publishes,
then hands off. The run is in status=awaiting_release. After you merge,
you can optionally mark the run as released.

**What you do with it:** Merge, deploy, ship.

---

## Quick Reference: The Full Sequence

Here's the complete flow for a feature with no remediation, showing what
tool you interact with at each step:

```
Step  | Tool    | What you do                          | What happens
------+---------+--------------------------------------+----------------------------------
 1    | Hermoso | hermoso init .                       | Project created
 2    | Hermoso | hermoso start <feature-id> .         | Run created (phase=design)
 3    | Hermes  | "Design a feature for [objective]"   | hermoso-design skill fires,
      |         |                                      | feature-design.json written
 4    | Hermes  | (automatic: design put)              | Design persisted in run state
 5    | Hermes  | (automatic: verification-author)     | Gherkin + contract written,
      |         |                                      | verification put, package sealed
 6    | Hermes  | "Approve"                            | approve design → phase=construction
 7    | Hermes  | (automatic: graph put)               | Work graph authored + persisted
 8    | Hermes  | (automatic: construction prepare)    | Cards compiled, worktrees created
 9    | Hermes  | (automatic: construction ready)      | Ready cards listed
10    | Kanban  | (workers dispatched)                 | Workers execute in worktrees
11    | Kanban  | (workers complete/block)             | Outcomes recorded via work start/complete
12    | Hermes  | (repeats 9-11 until all done)        | Multiple waves if DAG has depth
13    | Hermes  | "integrate"                          | Branches merged → phase=verification
14    | Hermes  | (automatic: verification run)        | Gherkin executed against feature branch
15    | Hermes  | (if rubric: judge/resolve)           | Judgments applied
16    | Hermoso | (automatic on pass: publish Gherkin) | Gherkin published → awaiting_release
17    | You     | git merge feature/<id> to main       | Feature shipped
```

With remediation, steps 7-16 repeat as round 2 (new work graph, new
construction, re-verify). Maximum one remediation round — a second
verification failure blocks the run.

---

## Quick Reference: Which Skill Fires When

```
Phase        Skill                       When
------------ --------------------------- ----------------------------------------
Design       hermoso-design              After "start" — user asks for a design
Design       hermoso-verification-author After "design put" — automatic, authors
                                         the hidden verification contract
Construction hermoso-construction        After "approve" — authors work graph,
                                         compiles cards, dispatches, integrates
Construction kanban-worker               Auto-injected into every dispatched
                                         worker agent session
Verification hermoso-verification        After "integrate" — runs verification,
                                         applies judgments, routes pass/fail
```

---

## Quick Reference: State Transitions

```
init          → project created
start         → design/pending
design put    → design/in_progress
verification put → design/awaiting_approval
approve       → construction/in_progress
graph put     → (construction round 1 created)
prepare       → (worktrees created, cards compiled)
work start    → (work item started)
work complete → (work item done)
work block    → construction/blocked
integrate     → verification/awaiting_verification
verification run → verification/in_progress (or awaiting_judgment)
verification judge/resolve (pass) → verification/awaiting_release
verification judge/resolve (fail) → construction/awaiting_remediation
verification remediate → construction/in_progress (round N+1)
(you merge to main) → released
```
