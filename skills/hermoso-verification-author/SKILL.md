---
name: hermoso-verification-author
description: "Use when a feature design is persisted to author the hidden hybrid verification contract, Gherkin, and verifier assets that complete one approvable Hermoso design package."
version: 1.1.0
author: Hermoso
license: MIT
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [hermoso, verification, bdd, contracts, traceability]
    related_skills: [hermoso, hermoso-design, hermoso-verification]
---

# Hermoso Verification Contract Author

## Boundary

Author the independent verification side of the design package before the work
graph exists. Build agents may see the approved feature design, but never send
them this contract, scenario IDs/text, verifier commands, fixtures, probes,
asset paths, or hashes. This is operational non-disclosure through context and
worktree separation, not an OS security boundary.

Never edit `.hermoso/**`. Never infer identity from cwd, conversation, branch
names, or assets. Pass explicit context (project-id, feature-id, run-id,
repository) to every `hermoso` command — the binary validates context against
persisted state and rejects mismatches. Call `hermoso context` to resolve
the canonical tuple when needed.

## Authoring flow

1. Refresh `status`, `context`, and the live schemas:

   ```sh
   hermoso schema feature-verification-contract --json
   hermoso model status <project-id> <repository> --json
   hermoso model query <project-id> <repository> orientation --json
   hermoso model query <project-id> <repository> evidence <surface-or-invariant-id> --json
   ```

2. Read the persisted v2 feature design and its exact `feature_hash` and
   `base_model`. Reject a stale model rather than guessing.
3. Trace every requirement and acceptance criterion to a judgment or an
   explicit, justified exclusion.
4. Use Gherkin for observable business behavior. Use deterministic commands,
   bounded property checks, or evidence-based rubrics when they are the better
   oracle. Do not force implementation details into scenarios.
5. Give every judgment stable requirement, criterion, surface, business-term,
   and invariant references. Commands are direct argv arrays; never wrap a
   shell command string.
6. Give property checks a fixed seed and bounded iteration count. Give rubrics
   a named judge, explicit criteria, and required evidence.
7. Hash every artifact, validate Gherkin tags and scenario IDs, and keep
   publication paths clean and repository-relative.
8. Write the contract and assets outside `.hermoso/**`, with artifact paths
   relative to the contract file, then ingest:

   ```sh
   hermoso validate feature-verification-contract <contract-path> <project-id> <feature-id> <run-id> <repository> --json
   hermoso verification put <project-id> <feature-id> <run-id> <repository> <contract-path> --json
   ```

`verification put` performs the stronger cross-contract checks, reads and
hashes the declared assets, seals them clone-locally, and returns the atomic
design-package revision/hash. Any design, contract, artifact, or model change
invalidates approval.

## Contract authoring rules (validator gotchas)

Learned from repeated validation rejections — check these before ingesting:

- **argv only, no shell wrappers.** `execution.command` must be direct argv.
  `["/bin/bash", "-c", "..."]` is rejected ("must execute argv directly
  rather than a shell command string"). `python -c` IS accepted (it is an
  interpreter argv, not a shell), but each argv value must be a single line —
  control characters (newlines) in argv values are rejected.
- **Multi-step orchestration goes in probe artifacts.** When a judgment needs
  service startup/teardown, env wiring, or sequencing, write it as an artifact
  with `kind: probe` (e.g. `probes/run_emulator_e2e.py`) and invoke it with a
  one-line runpy:
  `["<python>", "-c", "import os,runpy;runpy.run_path(os.path.join(os.environ['HERMOSO_VERIFICATION_ASSETS'],'probes/<name>.py'),run_name='__main__')"]`
- **Point commands at the interpreter that has the dependencies.** System
  `python3` usually lacks repo deps (flask, pytest...). Use a dedicated venv
  python absolute path in every judgment command, and make probe scripts use
  `sys.executable` for subprocesses so the venv propagates.
- **feature_design reference binds the CONTENT revision.** `verification put`
  cross-checks `feature_design.revision` against the design's content revision
  (the `revision` field inside the design file), NOT the wrapper revision the
  `design put` response reports (that one auto-increments and can differ).
  Read the persisted run state (`design.feature_design.revision` +
  `design.feature_hash`) before writing the reference.
- **Judgment required fields** (verify with the live schema each time):
  `id`, `title`, `modality`, `requirement_ids`, `acceptance_criterion_ids`,
  `surface_ids` (minItems 1), `execution`, `oracle`, `required_evidence`.
- **BDD judgments: one per scenario.** Each scenario's `@judgment` tag names
  its own judgment, and its `@requirement/@criterion/@surface/@term` tags must
  exactly match that judgment's ID lists. Every `@scenario` with a `@judgment`
  tag must appear in that judgment's `scenario_ids`. A single umbrella BDD
  judgment over many scenarios fails cross-validation.
- **Coverage is enforced.** Every requirement and acceptance criterion must be
  cited by at least one judgment, or listed in `coverage_exclusions` with a
  rationale (e.g. capability absent on the verification host — name the
  compensating judgments that cover the intent structurally). Missing coverage
  causes `verification put` to fail with "has no judgment or approved
  exclusion." For implementation-level requirements that can't be probed with
  shell scripts, use a Go-level unit test judgment (`go test ... -run TestX`).
- **Artifacts must be clean sub-paths of the contract directory.** `verification put`
  resolves artifact paths relative to the contract file and rejects `..`
  components as "not a clean relative path." Place probes, fixtures, and other
  assets in sub-directories under the same directory as the contract
  (`design/fixtures/`, `design/probes/`), never in a sibling directory.
- **Model snapshot must be current.** If the design's `base_model` references a
  stale snapshot (built at a prior commit), `design put` fails with "repository
  model snapshot is stale." Run `hermoso model build` first, then update
  `base_model` in both the feature-design and verification-contract JSON.
- **Environment defects are free to fix.** If you discover after sealing that
  a command targets the wrong interpreter or a missing env seam, use
  `hermoso verification amend` with a bumped contract revision — it re-seals
  without consuming a verification attempt and needs no re-approval. Do not
  burn an attempt on a known-bad command.
- **Design schema drift:** before authoring, re-read the live
  `feature-design` schema too. Requirements need `title`/`kind`/must-should-
  could `priority`; decisions are `{decision, rationale}` pairs; constraints
  are plain strings. A design authored against an older shape will not put.

## Gherkin tags

Every scenario has exactly one `@scenario:<id>` and one `@judgment:<id>`. Its
tags must agree with the JSON judgment:

- `@requirement:<id>`
- `@criterion:<id>`
- `@surface:<id>`
- `@term:<id>` when vocabulary is referenced
- `@invariant:<model-node-id>` when an invariant is referenced

Do not use placeholders, hidden examples that cannot be reproduced, or an
oracle that merely restates the implementation.

Before finalizing, verify no scenario contains unresolved placeholders
(TODO, TBD, NEEDS CLARIFICATION, [PLACEHOLDER]) in steps, names, or data.
The contract validator checks structural properties only — content quality
is the author's responsibility.

## Small Features

When the feature design's `complexity` is `small` (the bounded path), the
authoring ceremony shrinks. Produce ONLY BDD Gherkin scenarios and their
artifacts — no hand-authored deterministic judgments, no full
feature-verification-contract JSON. The deterministic side of the contract is
synthesized by Go from the work graph's `validation_commands`, so there is
nothing to hand-write for it.

Flow for `complexity: small`:

1. Write the Gherkin scenarios and scenario artifacts exactly as the full
   authoring flow requires (tags, IDs, hashes, publication paths).
2. Ensure a work graph exists whose work items carry `validation_commands`
   that cover every acceptance criterion. If the design flow has not produced
   one yet, author it now (the construction skill consumes the same graph).
3. Derive and persist the contract with `hermoso verification put --derive
   <work-graph>` instead of `put`ing a hand-written one:

   ```sh
   hermoso verification put <project-id> <feature-id> <run-id> <repository> --derive <work-graph> --json
   ```

   Hermoso reads the work graph, emits one deterministic `exit_code` (expected
   `0`) judgment per work item's `validation_commands`, merges the BDD
   judgments, and seals the result as the verification contract.
4. Present the package for approval exactly as in the full flow.

`--derive` is restricted to `complexity: small` — Go rejects it for
`standard` and `complex` designs, which keep the hand-authored contract from
the full flow. BDD Gherkin is still published and run at release unchanged.

The same boundary rules apply on the small path: never edit `.hermoso/**`;
never infer identity from cwd, conversation, branch names, or assets; pass
explicit context (project-id, feature-id, run-id, repository) to every
`hermoso` command and call `hermoso context` to resolve the canonical tuple
when needed.

## Completion

Present the human reviewer with the exact package revision/hash, coverage,
modalities, exclusions, and publication paths. The single approval is:

```sh
hermoso approve design <project-id> <feature-id> <run-id> <repository> <package-revision> <package-hash> <actor> [comment] --json
```

Do not disclose verifier assets to construction. On the standard and complex
paths the skill does not author the work graph; on the small path the work
graph is the derivation input (see Small Features above).
