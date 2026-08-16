---
name: hermoso-verification
description: "Use when construction reaches awaiting_verification to run the approved hidden contract read-only, interpret its persisted report, apply qualitative rubric judgments for pending outcomes, author remediation specs for failed attempts, route one remediation round, and publish passing Gherkin."
version: 1.1.0
author: Hermoso
license: MIT
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [hermoso, verification, bdd, evidence, remediation, rubric]
    related_skills: [hermoso, hermoso-verification-author, hermoso-construction]
---

# Hermoso Verification Lifecycle

## Entry and execution

Never infer identity from cwd, conversation, branches, or Kanban. Pass
explicit context (project-id, feature-id, run-id, repository) to every
`hermoso` command — the binary validates context against persisted state and
rejects mismatches. Call `hermoso context` to resolve the canonical tuple
when needed. Proceed only from
`construction/awaiting_verification` with the approved package unchanged.
Never edit `.hermoso/**` or materialize hidden assets in a build worktree.

### Phase 1 — Mechanical run

Run:

```sh
hermoso verification run <project-id> <feature-id> <run-id> <repository> --json
```

Hermoso then:

- verifies the exact integrated candidate commit and package hashes;
- creates an isolated verification worktree;
- materializes hash-locked assets outside construction worktrees;
- builds the candidate knowledge-spine snapshot and resolves planned surfaces;
- executes only approved argv commands with declared bounds;
- records command output, exit code, duration, seed, judge, and hashes;
- fingerprints the candidate before and after execution;
- persists a report even when setup or execution blocks.

The verdict from Phase 1 depends on the oracle type:

- **Gherkin oracle:** the BDD runner's exit code IS a deterministic oracle.
  Hermoso evaluates it mechanically and returns `pass` or `fail` immediately.
  No skill judgment is needed — proceed to Routing below.
- **Rubric oracle:** Hermoso records the evidence (command output, exit code,
  stdout/stderr) but does NOT evaluate quality. The outcome status is
  `judgment_pending` and the run transitions to `awaiting_judgment`.

Do not translate an infrastructure failure into a passing result. A completed
pass/fail accounts for every required judgment.

### Phase 2 — Surface resolution and qualitative judgment

When Phase 1 returns `awaiting_judgment`, the run has pending items that require
skill judgment. There are two types of pending items:

1. **Pending surface resolutions** — planned surfaces that Go could not
   deterministically match to model nodes (no cooperative `surface_id`
   tagging). The skill must determine whether each planned surface was
   implemented by the candidate.

2. **Pending rubric judgments** — rubric oracle outcomes that require
   qualitative assessment against rubric criteria.

Both can be present in the same run. Resolve them in any order — the run stays
in `awaiting_judgment` until the verdict is no longer pending.

#### Surface resolution

1. Read the Phase 1 report. Pending surface resolutions have `status: "pending"`.

2. For each pending surface, query the candidate model snapshot to find the
   corresponding node:
   ```sh
   hermoso model query <project-id> <repository> evidence <surface-or-invariant-id> --json
   ```

3. Determine whether the candidate implemented the planned surface. Write a
   `SurfaceResolution` entry for each pending surface:
   - `surface_id` — matches the pending resolution
   - `model_node_id` — the matching model node ID, or empty if unresolved
   - `status` — `"resolved"` if the surface was implemented, `"missing"` if not
   - `summary` — reasoned explanation of the match or non-match

4. Submit the resolutions:
   ```sh
   hermoso verification resolve <project-id> <feature-id> <run-id> <repository> <resolutions.json> --json
   ```

Go validates that all pending surfaces are resolved, applies the invariant
(unresolved surfaces fail linked judgments), and re-aggregates the verdict.

#### Qualitative judgment (rubric oracles only)

Steps:

1. Read the Phase 1 report from the JSON output. Each pending outcome has:
   - `judgment_id` — matches the verification contract's judgment definition
   - `evidence` — command output, exit code, stdout/stderr, hashes
   - `summary` — describes why the outcome is pending (rubric oracle)

2. For each pending outcome, apply the rubric criteria defined in the
   verification contract. Determine `pass` or `fail` and write reasoning.

3. Write a judgments JSON file with the following structure:

```json
[
  {
    "judgment_id": "<matches the pending outcome>",
    "status": "pass",
    "summary": "<one-line assessment>",
    "qualitative": {
      "judge": "rubric",
      "reasoning": "<detailed reasoning against rubric criteria>",
      "criteria": ["<criterion-id>", "..."]
    }
  }
]
```

4. Submit the judgments:

```sh
hermoso verification judge <project-id> <feature-id> <run-id> <repository> <judgments.json> --json
```

Hermoso then:

- validates that every pending outcome has a judgment with valid status;
- rejects any judgment that does not correspond to a pending outcome;
- applies the judgments, re-aggregates the verdict and coverage;
- transitions the run (pass → awaiting_release, fail → remediation or blocked).

Go enforces the contract (completeness, valid statuses, lifecycle transitions).
The skill provides the qualitative assessment (pass/fail, reasoning, criteria).

## Routing

- **Pass:** Hermoso publishes only approved Gherkin paths, commits them after
  proving the diff allowlist, refreshes the spine with stable scenario links,
  and transitions to `awaiting_release`.
- **Blocked publication after pass:** correct the external condition and run
  `hermoso resume ... --json`. Hermoso revalidates an existing publication
  commit before completing the spine refresh; do not rerun the behavioral
  judgments as a new attempt.
- **First fail:** Hermoso transitions to `construction/awaiting_remediation`.
  No remediation round is created yet — the skill must author it. Read the
  failed report from the JSON output (outcomes, evidence, summaries) and
  author a `SkillRemediation` spec with targeted needs and a work graph:

  1. For each failed outcome, write a `RemediationNeed` explaining what went
     wrong: `Expected` (what should have happened), `Actual` (what the
     evidence shows), and the affected requirement/criterion/surface IDs.
  2. Author a `WorkGraph` with work items that have specific, targeted prompts
     — not generic "fix the implementation" text. Include the correct worker
     profile and skill bindings. Set `Producer` to
     `{Skill: "hermoso-verification", Runtime: "agent"}`.
  3. Write the spec to a JSON file and submit:

  ```sh
  hermoso verification remediate <project-id> <feature-id> <run-id> <repository> <spec.json> --json
  ```

  Hermoso validates the spec structure, wraps it in a remediation round with
  deterministic fields (number, source hash, graph hash), and transitions to
  `construction/pending`. Then refresh `construction ready`, dispatch the
  visible cards, and follow `hermoso-construction`. The cards contain visible
  requirements, criteria, surfaces, and the skill-authored remediation
  guidance — not hidden scenarios or fixtures.
- **Second fail:** the run is blocked. Do not create a third round.
- **Test defect (amendment):** When verification failures are caused by bugs
  in the sealed test artifacts themselves — wrong API assumptions, incorrect
  mock patterns, wrong constructor signatures — not by defects in the
  construction code, author a corrected verification contract with the
  revision bumped and use:

  ```sh
  hermoso verification amend <project-id> <feature-id> <run-id> <repository> <contract-path> --json
  ```

  This re-seals the corrected artifacts, archives old attempts as incidents,
  resets the attempt counter, and transitions to `awaiting_verification`. No
  re-approval or re-construction is needed — the feature design is unchanged.
  After amending, run `verification run` for a fresh attempt. The new contract
  gets a fresh 2-attempt budget.

  Amendment is allowed from `verification/blocked` or
  `construction/awaiting_verification`. It does NOT consume a verification
  attempt — the old attempts tested a different (buggy) contract and are
  archived. To distinguish from remediation: remediation fixes the code,
  amendment fixes the tests.
- **Blocked/inconclusive:** surface the persisted findings for human action. If
  the external verifier condition is corrected, run `hermoso resume ... --json`
  and retry `verification run`; the incident report remains archived without
  consuming a semantic attempt. Contract defects or requirement changes return
  to redesign; do not mutate the approved contract during remediation.

The production verdict remains bound to the pre-publication candidate commit.
The later publication commit may contain only the approved Gherkin artifacts.

## Release Phase

After verification passes and Gherkin is published, the run is in
`awaiting_release`. To transition to `released`, run:

```sh
hermoso release <project-id> <feature-id> <run-id> <repository> [check-commands...] --json
```

The release command:

- Transitions through `Release/Pending → Release/InProgress → Release/Released`
- Optionally runs check commands (e.g. `go test ./...`, `behave features/`) in the
  feature worktree. Pass them as trailing arguments:
  ```sh
  hermoso release proj feat run . "go test ./..." "behave features/" --json
  ```
- On check failure, transitions to `Release/Blocked`. Retry with `hermoso resume`.
- After checks pass (or if no checks are given), merges the feature branch into the
  repository's default branch (e.g. `main`) with `--no-ff` to preserve the feature
  topology. The resulting merge commit is recorded as `release_commit` on the run.
- If the merge conflicts, transitions to `Release/Blocked`. Resolve conflicts in
  the root repo and retry with `hermoso resume`.
- On success, transitions to `Release/Released`. The default branch now contains
  the feature.

The BDD regression suite is the `features/` directory at the repo root, which
accumulates all published Gherkin from each feature via `publishPassingGherkin`.
At release, pass the BDD runner command as a check to catch regressions across
features — no separate copy of the `.feature` files is needed.

Feature artifacts (design, implementation notes, task notes) live in
`docs/features/[feature-id]/` in the target repo, created by `hermoso start`.
These are committed alongside code and visible to all developers.
