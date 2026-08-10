---
name: hermoso-verification-author
description: "Use when a feature design is persisted to author the hidden hybrid verification contract, Gherkin, and verifier assets that complete one approvable Hermoso design package."
version: 1.0.0
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

Never edit `.hermoso/**`. At entry and every handoff, run `hermoso context`,
compare the complete canonical tuple and absolute workspace, and block on any
mismatch. Never infer identity from cwd, conversation, branch names, or assets.

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

## Gherkin tags

Every scenario has exactly one `@scenario:<id>` and one
`@judgment:<id>`. Its tags must agree with the JSON judgment:

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

## Completion

Present the human reviewer with the exact package revision/hash, coverage,
modalities, exclusions, and publication paths. The single approval is:

```sh
hermoso approve design <project-id> <feature-id> <run-id> <repository> <package-revision> <package-hash> <actor> [comment] --json
```

Do not author the work graph and do not disclose verifier assets to
construction.
