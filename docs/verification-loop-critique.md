# Verification Loop: Skills vs. Go Binary Boundary Critique

Date: 2026-08-09
Branch: `bdd-contracts-and-verification`
Status: Design evaluation

## Principle

1. **Skills do value-added work**: reasoning, judgment, authoring, adaptive
   decisions, qualitative evaluation — anything where a model's judgment
   adds value over a fixed algorithm.
2. **The Go binary does orchestration and contract enforcement**: deterministic
   invariants, state transitions, hashing, validation, lifecycle boundaries,
   tamper detection — anything that must be trustworthy and reproducible.

The clean test: if removing the model from a step would make the output worse
(because a fixed template produces a worse result), that step should be a
skill. If removing the model from a step would make the result *more*
trustworthy (because a model can be talked out of an invariant), that step
should be Go.

## Where the design upholds the principle

### Go-owned (correct)

- **Hash-locked design packages** with optimistic concurrency in
  `persistVerificationAttempt` (attempt order, package hash, artifact root
  hash must all match). Exactly the kind of invariant a model must not
  override.
- **Report completeness** (`validateReportCompleteness`): every judgment must
  have an outcome, every requirement/criterion must be covered or excluded.
  Pure structural enforcement.
- **Tamper detection**: `worktreeFingerprint` before/after + commit immutability
  check. Fingerprint mismatch or commit drift forces `VerificationBlocked`.
- **Gherkin publication allowlist**: path escape prevention, content hash
  match, distinct commit requirement, rollback on failure.
- **Lifecycle limits**: two-attempt verification limit, one-remediation-round
  limit. Lifecycle boundary, Go-owned.
- **Coverage aggregation** with exclusions. Pure boolean logic.
- **Model spine**: Tree-sitter extraction, git inventory, component
  aggregation, test pairing, SCIP augmentation, incremental reuse.
  Deterministic code intelligence — Go owns it; skills query it.
- **Contract cross-validation** (`ValidateContract`): traceability, tag
  presence, artifact hash binding, surface/model-node kind matching.
  Structural enforcement, correct in Go.

### Skill-owned (correct)

- **`hermoso-design`**: adaptive depth (small/standard/complex), spike
  decision, work-graph proposal, approval presentation. All reasoning work.
- **`hermoso-verification-author`**: choosing modalities, writing Gherkin,
  selecting oracles, defining publication paths. Authoring work.
- **`hermoso-construction`**: authoring the work graph, card creation/binding,
  worker lifecycle, integration. Authoring + routing work.

## Where the design violates the principle

### Issue A (severe): Rubric/qualitative oracles are reduced to exit codes in Go

**Location**: `internal/workflow/verification.go`, `judgeExecution`, lines
358-416.

**Problem**: For `gherkin` and `rubric` modalities, the entire judgment is:

```go
case "gherkin", "rubric":
    if exitCode == 0 {
        return domain.JudgmentPass, "approved external judge passed"
    }
```

A rubric is a qualitative evaluation criterion. The entire point of a rubric
is that applying it requires reasoning — reading the candidate's output,
comparing it against multi-dimensional quality criteria, and forming a
judgment. By reducing this to `exitCode == 0`, the design does one of two
things:

- **(a)** The rubric is never actually applied by a reasoning agent. The
  external command's exit code IS the judgment, and Hermoso has lost the
  evidence trail — no structured reasoning, no transcript, just 0/1. The
  "rubric" is decorative.
- **(b)** The external command is itself an LLM call whose exit code is the
  verdict. In that case the reasoning happened, but outside Hermoso's contract
  envelope — no signed evidence, no structured outcome, no auditability. The
  model's reasoning is unattributed and unhashed.

Either way, the Go binary is pretending to apply a judgment while actually
outsourcing it to an opaque exit code. The `Producer` field says
`{Skill: "hermoso-verification", Runtime: "deterministic"}` — but for rubric
judgments there is no skill reasoning and no deterministic judgment, just a
process exit code.

**Recommendation**: Split the oracle evaluation into two tiers.

- **Mechanical oracles** (exit_code, stdout_regex, file, json_path) stay in
  `judgeExecution`. These need no reasoning.
- **Qualitative oracles** (rubric, and arguably open-ended BDD where the
  Gherkin runner's pass/fail is not sufficient) move to the
  `hermoso-verification` skill. The Go binary executes the verifier command
  to *collect raw evidence* (stdout, stderr, exit code, generated artifacts),
  persists that evidence deterministically, and then hands it to the skill.
  The skill applies the hash-locked rubric text via reasoning and produces a
  structured `JudgmentOutcome` with a verdict, a reasoned summary, and
  evidence references. The Go binary then validates the outcome's structure
  (correct judgment ID, valid status, evidence hashes match) and incorporates
  it into the signed report.

This keeps contract enforcement in Go (the rubric text is hash-locked, the
judge identity is approved, the evidence is collected deterministically)
while putting the actual qualitative judgment in the skill where reasoning
lives. The `Runtime` field becomes "agent" for these outcomes and
"deterministic" for mechanical ones — honestly reflecting what produced each
verdict.

### Issue B (severe): The remediation spec is templated by Go, not authored by a skill

**Location**: `internal/workflow/verification.go`, `remediationRound`, lines
575-645.

**Problem**: Go mechanically maps each failed judgment to a remediation need
and a work item with a hardcoded prompt:

```go
Prompt: "Correct the implementation to satisfy the referenced visible requirements and acceptance criteria. " +
    "Observed summary: " + outcome.Summary,
```

This is value-added authoring work. A remediation need should explain *what
specifically went wrong* — which assertion failed, what the candidate
produced vs. what was expected, which surface was missing, what the likely
root cause is. A templated string that says "Correct the implementation to
satisfy the referenced visible requirements" is the kind of generic guidance
that adds no value over the failure summary itself.

Compare this to how the system handles initial construction: the
`hermoso-design` skill authors the work graph with adaptive depth and
specific guidance, then Go validates and persists it. Remediation should
follow the same pattern:

- **Go detects** the failure, enforces the two-attempt limit and
  one-remediation-round limit, and exposes the failed report + evidence +
  affected surfaces to a skill.
- **The skill authors** the remediation spec: reads the failure evidence,
  the failed judgment summaries, the affected surfaces, and the original
  design, then writes targeted remediation needs with specific guidance and
  a right-sized remediation graph (which might be one item or several, might
  have dependencies, might need a spike).
- **Go validates** the skill-authored spec (same `WorkGraph.Validate()` that
  initial construction uses) and persists it.

The current design has Go both authoring and persisting, which produces
low-quality remediation guidance and duplicates the construction-authoring
responsibility that already lives in skills. The `Producer` on the
remediation graph is
`{Skill: "hermoso-verification", Runtime: "deterministic"}` — a category
error: a skill credited as producer but with deterministic runtime means no
reasoning happened.

**Recommendation**: Add a skill step (in `hermoso-verification` or a dedicated
`hermoso-remediation-author`) that authors the remediation spec from the
failed report. The Go binary exposes
`hermoso verification remediate <context> --json` which returns the failed
report + evidence and accepts a skill-authored remediation spec for
validation and persistence, mirroring the `design put` / `graph put` pattern.

### Issue C (moderate): Planned surface resolution is a fuzzy title match in Go

**Location**: `internal/workflow/verification.go`, `resolveCandidateSurfaces`,
lines 437-474.

**Problem**: "Planned" (not-yet-existing) surfaces are resolved by:

```go
node.Attributes["surface_id"] == surface.ID || strings.EqualFold(node.Title, surface.Title)
```

"Did the candidate actually implement the planned API surface?" is a semantic
question. A surface titled "User registration endpoint" in the design could
be implemented as `POST /v1/signup` with a different node title — the Go
matcher reports it missing even though it exists. Conversely, a node with a
matching title but wrong semantics passes. The `surface_id` attribute match
only works if the construction worker cooperatively tagged their node, which
is not enforced.

The *invariant* — unresolved surfaces cause linked judgments to fail (lines
131-145) — is correctly in Go. But the *resolution itself* — "does this
model node correspond to this planned surface?" — is a judgment. A reasoning
agent with access to the candidate model snapshot and the design surfaces
would do this far better than `strings.EqualFold` on titles.

**Recommendation**: The Go binary builds the candidate model snapshot (as it
does now) and exposes it alongside the design surfaces. The
`hermoso-verification` skill produces `SurfaceResolution` entries with
reasoned summaries. Go validates that all surfaces are resolved (the
invariant) and overrides linked judgment outcomes to fail if not. The fuzzy
title match becomes a fallback only when no skill resolution is provided, or
is removed entirely.

### Issue D (moderate): Identity verification is a skill ritual instead of a binary invariant

**Location**: All four skills (`hermoso`, `hermoso-design`,
`hermoso-construction`, `hermoso-verification`).

**Problem**: Every skill repeats the instruction: "refresh context, echo
project/feature/run/repository, compare all fields and the absolute workspace,
and block on any mismatch." This asks a reasoning model to manually verify
identity — compare four strings and a path — at every phase boundary.

This is error-prone when done by a model: the model can forget to echo,
compare wrongly, or be convinced by conversational context that a mismatch
doesn't matter. Identity binding is a contract; the Go binary should enforce
it, not the skill.

**Recommendation**: Every `hermoso` CLI command already takes the full
context tuple (project-id, feature-id, run-id, repository) as explicit
arguments. The binary should validate this tuple against persisted state on
*every* command and return a structured error on mismatch — making it
impossible to operate on the wrong context. Then remove the echo-and-compare
ritual from the skills. The skill just calls the command and checks `ok`; if
`ok` is false with a context mismatch error, it blocks.

If the binary already does this (the `s.Store.Run(ctx, execution)` call loads
by context ref and would presumably fail on a wrong run ID), then the skill
ritual is redundant ceremony that adds failure modes — the model might echo
correctly but the binary would have caught it anyway, or the model might
echo incorrectly and block when the binary would have succeeded. Either way,
the skill is doing enforcement that the binary either already does or should
do.

### Issue E (minor): Gherkin placeholder scan is a content-quality check in the contract validator

**Location**: `internal/verification/contract.go`, `parseGherkin`, lines
254-258.

**Problem**: The parser scans for uppercase TODO/TBD/NEEDS CLARIFICATION/
[PLACEHOLDER] and rejects the artifact. This is a fragile proxy — it would
flag a legitimate Gherkin step like `Given the user types "TODO" into the
field`. Content quality (no placeholders, meaningful steps, good scenario
naming) is the `hermoso-verification-author` skill's job at authoring time.
The contract validator should check *structural* properties (parseable
Gherkin, exactly one @scenario tag, unique scenario IDs, required tag
presence) — which it does correctly — and leave content-quality screening to
the authoring skill.

**Recommendation**: Remove the placeholder scan from `parseGherkin`. If a
placeholder check is desired, add it as a skill-level authoring checklist
item in `hermoso-verification-author`, not as a Go contract gate.

## Summary

The verification loop gets the structural boundary right: Go owns hashing,
persistence, state transitions, attempt limits, coverage completeness,
publication allowlists, tamper detection, and model building. The skills own
authoring (design, contract, work graph) and routing.

It gets the judgment boundary wrong in three places, all in
`internal/workflow/verification.go`:

| Function | What Go does | What it should do | Who should do the judgment |
|---|---|---|---|
| `judgeExecution` (rubric/BDD) | Reduces qualitative oracle to exit code | Collect evidence, let skill apply rubric | `hermoso-verification` skill |
| `remediationRound` | Templates remediation needs + work items | Enforce limits, expose failure, accept skill-authored spec | `hermoso-verification` or remediation-author skill |
| `resolveCandidateSurfaces` (planned) | Fuzzy title match | Enforce "all surfaces resolved" invariant, let skill resolve | `hermoso-verification` skill |

And it has the inverse problem in one place:

| Function | What the skills do | What Go should do |
|---|---|---|
| Identity verification (all skills) | Manually echo/compare context at every boundary | Reject mismatched context on every command; remove the ritual |

### The throughline

The `hermoso-verification` skill is currently too thin — it calls
`hermoso verification run` and routes the result. It should be the agent that
applies rubric judgments, resolves planned surfaces against the model, and
authors remediation specs. The Go binary should collect evidence, enforce
invariants, validate structure, and persist — but stop short of making
qualitative judgments that a reasoning model would make better.
