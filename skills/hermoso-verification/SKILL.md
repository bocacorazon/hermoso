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

### Phase 2 — Qualitative judgment (rubric oracles only)

When Phase 1 returns `awaiting_judgment`, the skill must apply rubric criteria
to the collected evidence. This is the value-added judgment work that belongs
in the skill, not in Go.

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
- **Blocked/inconclusive:** surface the persisted findings for human action. If
  the external verifier condition is corrected, run `hermoso resume ... --json`
  and retry `verification run`; the incident report remains archived without
  consuming a semantic attempt. Contract defects or requirement changes return
  to redesign; do not mutate the approved contract during remediation.

The production verdict remains bound to the pre-publication candidate commit.
The later publication commit may contain only the approved Gherkin artifacts.
