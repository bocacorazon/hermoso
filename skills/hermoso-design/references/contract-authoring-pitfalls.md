# Contract Authoring Pitfalls (Design Phase)

Lessons from the first end-to-end design session using the v3.0.0
path-branching process (hermoso-constitution feature, 2026-08-21).

## schema_version Must Be "2" (String)

The feature-design JSON `schema_version` field must be the string `"2"`,
not `"hermoso-feature-design/v2"` or any other format. The validator
rejects anything else with `must be "2"`.

Same applies to the verification contract: `schema_version: "2"`.

## vocabulary_version Must Be "v1"

The `base_model.vocabulary_version` field must match the model snapshot's
manifest value. The model builder (`internal/model/build.go`) sets this to
`"v1"` — not a date, not a semantic version. Check with:

```sh
python3 -c "import json,glob; f=glob.glob('.hermoso/model/snapshots/*/manifest.json')[0]; d=json.load(open(f)); print(d.get('vocabulary_version'))"
```

The `current.json` file does NOT expose `vocabulary_version` — only the
snapshot manifest does. Always read the manifest, not `current.json`.

## Existing Surfaces Require Non-Empty model_node_id

The feature-design validator requires `source: "existing"` surfaces to
have a non-empty `model_node_id` referencing a real model spine node. But
the Tree-sitter model spine only produces file-level nodes for Go, JS, TS,
and Python files — NOT markdown files. SKILL.md and other .md files have
no model nodes.

**Fix:** For files the model spine can't see (markdown, YAML, etc.), use
`source: "planned"` instead of `"existing"`. The modification you're
planning to make IS a planned change — the file exists but the spine can't
reference it.

## unresolved_questions Must Be Empty

The validator rejects `design put` when `unresolved_questions` is
non-empty. Every question must be resolved before persisting the design.
If a question is genuinely unresolved, it must be moved to the design doc's
"Open Questions" section or resolved with a decision before contract
authoring.

## Verification Contract Hash Must Match design put Output

The verification contract's `feature_design.hash` field must match the
actual `feature_hash` returned by `hermoso design put`. The workflow:

1. Author feature-design JSON with a placeholder hash
2. Validate: `hermoso validate feature-design ...`
3. Persist: `hermoso design put ...` → captures `feature_hash` from output
4. Update the verification contract with the real hash
5. Validate: `hermoso validate feature-verification-contract ...`
6. Persist: `hermoso verification put ...`

Skipping step 4 (leaving the placeholder hash) causes `verification put`
to fail with `does not bind the exact feature design revision and hash`.

## Stale Submodule Gitlink Entries Block Model Build

`hermoso model build` fails with `parse Git tree entry "160000 commit ..."`
when the git index contains stale submodule (gitlink) entries — directories
that were committed as submodules but no longer have `.gitmodules` mappings
or checked-out submodules.

This happened with `.hermoso-test/fixture-*` directories left from test
runs. `git rm --cached` fails on these because they're gitlinks, not
regular files. Fix:

```sh
# Stage for deletion using update-index (works on gitlinks)
for f in $(git ls-tree HEAD <path>/ | awk '{print $4}'); do
  git update-index --force-remove "$f"
done
git commit -m "chore: remove stale submodule gitlink entries"
```

After cleanup, `hermoso model build` succeeds.

## Model Query Modes

`hermoso model query` supports `orientation` mode (returns high-level
component and repository nodes). The `design` query mode referenced in
the skill does NOT exist — use `orientation` instead.

## Model Snapshot Must Be Fresh Before design put

`hermoso design put` checks that the contract's `base_model.snapshot_id`
references a snapshot whose `source_revision` matches HEAD. If the
model was built at an older commit and new commits exist, it fails:

> repository model snapshot is stale: indexed \<old-rev\>, repository HEAD is \<new-rev\>

**Fix workflow:**

1. Run `hermoso model build <project-id> <repository> --json` first
2. Extract the new `snapshot_id`, `content_hash`, and `source_revision`
   from the output
3. Update `base_model` in **both** `feature-design.json` and
   `feature-verification-contract.json` to the fresh values
4. Re-validate both contracts before `design put` / `verification put`

## Verification Contract Artifacts Must Live Under the Contract Directory

`hermoso verification put` resolves artifact paths relative to the
contract file's directory, then validates them — it rejects paths
containing `..` components as "not a clean relative path."

**Correct layout:** place probes, fixtures, and other artifacts under
the same directory as the verification contract:

```
design/
  feature-design.json
  feature-verification-contract.json
  fixtures/create-telemetry-db.py
  probes/probe-usage-default.sh
```

Artifact paths in the contract then use clean sub-directory paths:
`"fixtures/create-telemetry-db.py"`, `"probes/probe-usage-default.sh"`.
Do NOT place verification artifacts in a sibling directory next to
`design/` — the only way to reach them would be `../verification/...`
which the path validator rejects.

## Verification Contract Must Cover Every AC and Requirement

`hermoso verification put` cross-validates that every acceptance criterion
and every requirement in the feature design has at least one judgment
(or explicitly approved exclusion). Uncovered items cause rejection:

> acceptance criterion \"ac-foo\" has no judgment or approved exclusion;
> requirement \"req-bar\" has no judgment or approved exclusion

**Fix:** add a judgment entry for each uncovered AC/requirement in the
verification contract's `judgments` array. For implementation-level
requirements that can't easily be probed with shell scripts, use a
Go-level unit test judgment:

```json
{
  "id": "judgment-backfill-labeled",
  "title": "Coarse backfill rows labeled estimated",
  "modality": "deterministic",
  "requirement_ids": ["req-coarse-backfill"],
  "acceptance_criterion_ids": ["ac-backfill-labeled-estimated"],
  "execution": {
    "command": ["go", "test", "./internal/telemetry/...", "-run", "Backfill", "-v"],
    "timeout_seconds": 60
  },
  "oracle": {"type": "exit_code", "expected": "0"}
}
```

## Check design put Output for verification put Hash

The verification contract's `feature_design.hash` must use the exact
`feature_hash` from `hermoso design put` output, NOT the local file's
sha256 sum. The two differ because `design put` computes the hash over
the canonical serialized form (with specific ordering), not the raw
file bytes.

After `design put` succeeds, extract `data.design.feature_hash` from
the JSON output and use it in the verification contract's
`feature_design.hash` field.

## approve design Requires Full Hash Prefix

`hermoso approve design` requires the package hash WITH the `sha256:`
prefix, not just the hex digest. Passing the bare hex digest fails with
`approval revision or hash does not match current design package`.

## Gherkin BDD Scenarios Require Strict Bidirectional Tag Matching

The verification contract cross-validator enforces that every BDD
scenario's gherkin tags exactly match its judgment's reference lists:

- Every scenario declared in a judgment's `scenario_ids` must carry:
  - `@requirement:<id>` for each of the judgment's `requirement_ids`
  - `@criterion:<id>` for each of the judgment's `acceptance_criterion_ids`
  - `@surface:<id>` for each of the judgment's `surface_ids`
  - Exactly one `@judgment:<judgment-id>` tag (no more, no less)
- Every scenario in the gherkin file must appear in its judgment's
  `scenario_ids`.
- Tags placed before the `Feature:` line apply to the feature element and
  propagate to ALL scenarios, causing "must have exactly one @judgment"
  errors. Put tags only before individual `Scenario:` lines.

Missing any single tag causes dozens of `scenario X is missing @criterion:Y`
errors. The cross-validator checks completeness in both directions.

## working_directory "." Is Invalid — Omit It

`execution.working_directory` rejects `"."` as an invalid relative path.
When you want the workspace root, omit the `working_directory` field
entirely — it defaults to the workspace root. Only set it when you need
a specific subdirectory.

## Coverage Exclusions Need Separate Entries per target_kind

When a requirement AND its acceptance criteria both lack judgments,
add separate `coverage_exclusions` entries for each `target_kind`:

```json
{"target_kind": "requirement", "target_id": "req-foo", "rationale": "..."},
{"target_kind": "acceptance_criterion", "target_id": "ac-foo", "rationale": "..."}
```

A single requirement exclusion does NOT cover its acceptance criterion —
the validator checks both independently.

## New Schema Fields Rejected Until the Binary Supports Them

When a domain type gains a new field (e.g., `max_verification_attempts`
on `FeatureVerificationContract`), the current binary's schema validator
rejects it as `json: unknown field "max_verification_attempts"` until
construction adds it. During the design phase, strip the field from the
contract JSON. Record it in the feature-design's `constraints` array as a
construction-time TODO, and add it back during construction before sealing.
