# Graduated Verification — Design

## Feature summary

Hermoso's verification ceremony is uniform today: every feature, from a one-line
flag to a new lifecycle phase, must hand-author a full verification contract with
sealed artifacts and judgments. The hard two-attempt verification cap (fail once →
remediate → fail again → blocked) is also fixed. This feature makes both **graduated**:

1. **Part A — derived contracts for `complexity: small`:** Go synthesizes the
   verification contract from work-graph `validation_commands` instead of the
   skill hand-authoring deterministic judgments. The skill still authors BDD
   scenarios (Gherkin is published and run at release), but the "author a
   verification contract" phase shrinks to "produce BDD artifacts + work graph,
   then `verification put --derive`."
2. **Part B — configurable attempt cap:** the hard two-attempt limit becomes
   `max_verification_attempts` (default 2) on the verification contract, sealed at
   design time and part of the immutable design package.

## Complexity assessment

**Complex.** This spans the lifecycle state machine (`internal/workflow/verification.go`
attempt branches), domain validation (`internal/domain/lifecycle.go`,
`verification_report.go`), the CLI surface (`internal/app/app.go`), the design/verification
skills, and the constitution. It also introduces a new deterministic derivation
engine in Go. Cross-package, new domain concept, multi-session.

## Clarifications

| Question | Decision |
|---|---|
| One feature or two? | One feature, two parts, shipped together |
| Principle II tension resolution | **Amend the constitution** — small features may substitute work-graph validation commands for a full contract |
| Remediation loop bound | **Iteration count** — configurable `N` remediation rounds (replaces hard 2-attempt cap) |
| Small features still publish BDD? | **Yes** — BDD still published and run; only the authoring ceremony shrinks |
| Part A architecture | **Go-enforced derivation** — Go synthesizes the contract from validation commands (derivation engine) |

## Alternatives considered

**A (chosen): Go-enforced derivation.** Go synthesizes deterministic judgments from
work-graph `validation_commands` for `complexity: small`. No hand-authored deterministic
judgments exist; `design approve` validates the derived contract covers all acceptance
criteria. Most deterministic; a `small` feature can never accidentally grow heavy ceremony.
Cost: a new derivation engine to maintain.

**B: Skill-owned with Go guardrails.** The skill still authors the minimal contract; Go
only blocks overbuilt ones (judgment count > validation-command count) for `small`.
Less Go, but no deterministic guarantee against ceremony creep.

**C: Pure skill.** Go untouched for ceremony; only the attempt cap is added. Fastest to
ship, but no code layer catches skill regression.

## Design overview

### Part A — Derived verification contracts

`FeatureVerificationContract` gains an optional `max_verification_attempts`
(`*uint64`, default 2 when absent, validated `>= 1`).

A new Go function `DeriveVerification(featureDesign, workGraph)` synthesizes a
`FeatureVerificationContract`:

- One `VerificationJudgment` per work item with a non-empty `validation_commands`.
  Oracle = `exit_code` with expected `"0"`; modality = `deterministic`.
- Judgment `requirement_ids` / `acceptance_criterion_ids` / `surface_ids` mirror the
  work item's own, so coverage validation stays consistent.
- BDD judgments authored by the skill are merged in (via the `--bdd` partial-contract
  flag); deterministic judgments are derived, not hand-written.
- The derived contract still carries BDD Gherkin artifacts, so publication and the
  release-phase regression suite run unchanged.

`hermoso verification put` gains a `--derive <work-graph-path>` flag plus an optional
`--bdd <partial-contract-path>`. With `--derive`, Go reads the work graph, synthesizes
deterministic judgments, merges the BDD partial, and persists the result as the sealed
verification contract.

The ceremony shrink lives in the skills: for `complexity: small`, `hermoso-design`
skips the separate verification-contract authoring sub-phase and `hermoso-verification-author`
produces only BDD artifacts + the work graph, then calls `--derive`.

### Part B — Configurable attempt cap

`max_verification_attempts` replaces the hardcoded `2` in:

- `internal/workflow/verification.go:42` — `len(run.VerificationAttempts) >= 2` → `>= contract.MaxAttempts`
- `internal/workflow/verification.go:309-315` — `attempt.Number == 1` remediation branch → `attempt.Number < maxAttempts`
- `internal/workflow/verification.go:699-705`, `:980-985` — same in `PutSurfaceResolutions` and `JudgeVerification`
- `internal/domain/lifecycle.go:308` — `attempt.Number > 2` → `> maxAttempts`
- `internal/domain/verification_report.go:145` — `r.Attempt > 2` → `> maxAttempts`

`VerificationReport` carries its own `MaxAttempts` (or the validation is relaxed to
`attempt > 0` and the workflow layer enforces the cap), so the report stays
self-validating. Amendment (`internal/workflow/amend.go`) already resets the budget by
clearing `VerificationAttempts` — no change needed; the new contract's
`max_verification_attempts` governs the fresh budget.

### Constitution amendment (Principle II)

Principle II gains: features classified `complexity: small` may derive their verification
contract from work-graph validation commands rather than a hand-authored contract.
Larger features keep the hand-authored contract. The escape-hatch override remains for
genuine bypass. This is a MINOR version bump (material expansion, no incompatible
removal).

## Verification approach

BDD scenarios cover: (1) a `small` feature derives a contract whose judgments match its
work-graph validation commands and whose coverage matches the acceptance criteria;
(2) a `small` derived contract still publishes Gherkin and runs the release regression;
(3) `max_verification_attempts=1` blocks a second attempt; `=4` allows up to four
attempts with remediation between failures; (4) absence of the field defaults to 2
(backward compatible); (5) amendment resets the budget regardless of cap value.

## Open questions

- Should `--derive` be restricted to `complexity: small` designs (Go blocks it for
  `standard`/`complex`), or allowed generally? *Leaning: restrict to `small`.*
- Default cap for derived `small` contracts: 1 (no remediation) or 2? *Leaning: keep 2
  for consistency, but expose the field so the skill can lower it.*
