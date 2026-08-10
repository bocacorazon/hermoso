# Feature verification contracts

Hermoso creates a verification contract during design, before the construction
work graph. The contract defines how the feature will be judged, traces every
visible requirement to repository interaction surfaces, and gives construction
and verification one immutable target.

## Why hybrid rather than BDD-only

BDD/Gherkin is the default for observable business behavior and shared
vocabulary. It should not replace lower-level unit/integration tests or force
non-behavioral qualities into scenarios. A contract may therefore combine:

- **BDD:** user-visible behavior exercised through an API, CLI, UI, file, event,
  or library surface;
- **deterministic checks:** objective commands, protocol probes, and file or
  structured-output oracles;
- **bounded property checks:** generated cases with a recorded seed, iteration
  limit, and failure oracle;
- **rubrics:** evidence-based judgment only where no objective oracle exists,
  with an explicit judge and criteria.

## Design package

The visible `feature-design` v2 document contains stable requirement and
acceptance-criterion IDs, business vocabulary, existing/planned surfaces, and
an exact immutable repository-model snapshot. The separate hidden
`feature-verification-contract` contains judgments, Gherkin, commands, probes,
fixtures, evidence requirements, aggregation, exclusions, and publication
paths.

`hermoso verification put` cross-validates both documents and seals all
referenced assets in clone-local content-addressed state. One design approval
binds:

1. feature-design hash;
2. verification-contract hash;
3. sealed-artifact root hash;
4. repository-model snapshot;
5. canonical package revision/hash.

Changing any input invalidates approval. Hiding is an operational boundary:
build contexts and worktrees do not receive hidden assets, but Hermoso does not
claim adversarial OS-level secrecy.

## Traceability

Every judgment cites requirement, acceptance-criterion, and surface IDs. It may
also cite business terms and model invariant nodes. Every requirement and
criterion is either covered or has an explicit approved exclusion.

BDD scenarios use stable tags:

```gherkin
@requirement:req-create @criterion:ac-created
@surface:surface-create @term:term-record
Feature: Create a record

  @scenario:scenario-create @judgment:judgment-create
  Scenario: Create a valid record
    Given no record exists
    When the user creates a record
    Then the record is available
```

The JSON judgment must contain the same IDs. Gherkin parse errors, duplicate
scenario IDs, tag drift, placeholders, path traversal, hash changes, shell
command strings, unbounded properties, and incomplete rubrics are rejected.

A corresponding JSON fragment is:

```json
{
  "id": "judgment-create",
  "title": "Create a record",
  "modality": "bdd",
  "requirement_ids": ["req-create"],
  "acceptance_criterion_ids": ["ac-created"],
  "surface_ids": ["surface-create"],
  "business_term_ids": ["term-record"],
  "artifact_ids": ["artifact-create"],
  "scenario_ids": ["scenario-create"],
  "execution": {
    "command": ["go", "test", "./features/..."],
    "timeout_seconds": 300
  },
  "oracle": {"type": "gherkin"},
  "required_evidence": ["scenario result", "command output"]
}
```

## Lifecycle

```text
design + hidden verification contract
  -> one package approval
  -> initial construction
  -> verification attempt 1
     -> pass: publish approved Gherkin, refresh spine, awaiting_release
     -> fail: one sanitized remediation specification and construction round
        -> verification attempt 2
           -> pass: publish and refresh
           -> fail: blocked
     -> blocked/inconclusive: human correction, resume, retry same attempt
```

Verification runs in an isolated worktree at the exact integrated commit.
Commands receive sealed assets through `HERMOSO_VERIFICATION_ASSETS`. Hermoso
records argv, working directory, output, exit code, duration, hashes, property
seed/iterations, and rubric judge; it blocks if the candidate worktree or commit
changes.

Every attempt persists a report with judgment outcomes, aggregated requirement
and criterion coverage, surface resolution, findings, evidence, exact model and
commit references, and final verdict. Remediation cards contain only visible
requirements, criteria, surfaces, and sanitized expected/actual behavior.

After a pass, Hermoso commits only hash-locked Gherkin at approved paths. The
production verdict remains bound to the preceding code commit. A new spine
snapshot at the publication commit promotes scenario nodes to `stable` and
links them to requirements, concepts, observed interfaces, and invariants.
