# Tiered Model Strategy: Local + Frontier Models for Hermoso

> **Supersedes:** FINAL-SYNTHESIS.md (local-only model assignments)
> **Date:** 2026-08-15
> **Foundation:** 9 benchmark runs (001-009) across 7 evaluation dimensions
> **New context:** User has OpenRouter subscription — frontier models available, cost-constrained not time-constrained

---

## 1. Executive Summary

The local-only benchmark proved that 5 local models can cover most coding tasks, but three blind spots remain: deep race-condition analysis, multi-turn signature preservation, and architectural judgment on ambiguous specs. Frontier models (via OpenRouter) can fill these gaps without replacing local models for high-volume work.

**The strategy: local models generate, frontier models verify.** 80-90% of token volume stays on local hardware (free). Frontier models see only diffs, reviews, and judgments — not full file contents. This keeps cost low while catching the blind spots that all local models share.

Hermoso's current 5 lifecycle roles are too coarse for per-task model assignment. This document decomposes them into 12 granular sub-agents, each with a tier-appropriate model.

---

## 2. Model Inventory

### Local Models (Tier 0 — $0 marginal cost)

| Shorthand | Full name | Type | Active params | TPS | Port | Strength |
|-----------|----------|------|--------------|-----|------|----------|
| qwen3-coder | qwen3-coder-30b | MoE | 3B | 98-156 | 8083 | Speed, docs, editing |
| qwen-64k | qwen-35b-a3b-64k | MoE | ~3.5B | 29-35 | 8080 | Architecture, test pass rate |
| qwen-27b | qwen-qwen2.5-27b | Dense | 27B | 25-31 | 8081 | Reliability, multi-turn coherence |
| gemma | gemma-4-31b-it | Dense | 31B | 20-28 | 8082 | Balance, concise judgments |
| qwen-3.8-27b | Qwen3.8-27B (thinking) | Dense | 27B | 20-26 | 8084 | Correctness (best), multi-turn, docs — at 10-40× latency |

**Constraint:** Only one model fits in 32GB VRAM at a time. Switching requires stop/start (~15s). Single-slot llama.cpp serializes all requests — no concurrent inference.

> **qwen-3.8-27b is a thinking model** (emits an explicit `reasoning_content`
> stream). It is **batch-only** — not for interactive use. Its benchmark profile:
>
> | Dimension | Result (vs the 4 Qwen 3.6 models) |
> |-----------|-----------------------------------|
> | Go from-scratch | **35/35 tests + 3/3 docs** — best correctness, but 23.5 min |
> | Multi-turn | **5/5 turns** — only model to avoid the signature-change trap |
> | Edits | 4/4 correct, but 393s/edit (39× slower than qwen3-coder) |
> | Debugging | 4/6 FULL + 1 PARTIAL + 1 timeout (reasoning runaway on TDD prompt) |
> | Long context | correct at 20K/40K, but 47-54 min (re-reasons the whole haystack) |
> | Long output | truncates at 32K tokens (Java port) — reasoning eats the budget |
>
> **Net:** a correctness specialist for batch work — the judge and the
> correctness-critical generator below — never for anything interactive.

### Frontier Models (via OpenRouter — pay per token)

| Tier | Example models | Cost range | Use when |
|------|---------------|------------|----------|
| 1 (Cheap) | claude-haiku, gpt-4o-mini, gemini-flash, deepseek-chat | ~$0.25-1/M tok | Code review, guardrail checks, test adequacy, signature-change detection |
| 2 (Mid) | claude-sonnet, gpt-4.1, deepseek-v4 | ~$3-15/M tok | Design review, rubric judgment, remediation spec authoring, race-condition analysis |
| 3 (Premium) | claude-opus, o3, gpt-4.1-pro | ~$15-75/M tok | Complex architecture decisions, cross-component design, final approval gate |

**Note:** Specific model selection deferred to user. The tiers are defined by capability and cost, not brand. Any OpenRouter model that fits the tier's capability profile works.

---

## 3. Tiered Assignment Principles

### The Tandem Pattern

```
Local model generates  →  Frontier model reviews  →  Local model remediates
     (Tier 0)                (Tier 1 or 2)              (Tier 0)
```

- **Generation stays local:** Code writing, test writing, doc writing — all on local models. This is 80-90% of token volume.
- **Verification goes frontier:** Code review, guardrail enforcement, qualitative judgment — on cheap or mid frontier models. These tasks need reasoning depth that local models lack, but they process small inputs (diffs, not full files).
- **Escalation is explicit:** If a frontier verifier flags a deep issue (race condition, architecture flaw), it goes to a mid-tier model for analysis, or to human.
- **Wall-clock is accepted, not optimized:** The user explicitly accepts longer cycles. A feature that takes 30 minutes with tandem local+frontier is fine if it costs $0.50 instead of $5.00.

### What Frontier Models Fix

From the benchmark, three blind spots that no local model crosses:

1. **Race-condition depth** (run-006 Bug B): All 5 local models — including the thinking model qwen-3.8-27b — add the obvious mutex but miss the JSON-encoder-reads-after-unlock race. A frontier model reviewing the diff can catch this.
2. **Signature-change trap** (run-007 turn 5): Four of five local models modify existing function signatures instead of adding new methods. **qwen-3.8-27b is the exception** — its thinking mode checked for existing callers and cleared all 5 turns. Still, a frontier code reviewer catches this cheaply and reliably without the 23-min-per-turn latency qwen-3.8-27b pays for it.
3. **Over-engineering on open prompts** (run-006 qwen-64k): Local models invent new API shapes under open-ended diagnosis prompts. A frontier design review before construction catches ambiguous specs.

### What Stays Local

- All code generation (greenfield + edits) — local models are fast and accurate enough
- Test writing — local models with TDD prompts are focused and fast
- Documentation — qwen3-coder writes the best docs (3/3 score)
- Remediation first-pass — qwen3-coder at 5s/fix is ideal for rapid cycles
- Context processing up to 40K — all local models handle this

---

## 4. Granular Hermoso Agent Decomposition

Current 5 lifecycle roles → 12 granular sub-agents. Each sub-agent has a single responsibility and a model assignment that fits its requirements.

### Design Phase

Currently: `hermoso-design` (one agent does everything)

#### Agent D1: Design Architect

**Responsibility:** Spine-grounded feature design — requirements, acceptance criteria, vocabulary, interaction surfaces. Reads the knowledge spine, queries model nodes, produces the feature-design artifact.

**Model:** Tier 2 (mid frontier) for complex features. Tier 0 local (qwen-64k) for simple features — it has the best architecture sense locally.

**Why:** Design is the foundation. Errors here propagate through construction and verification. A mid-tier frontier model catches ambiguity and architectural mistakes that local models miss. For simple, well-specified features, qwen-64k's architecture quality (best in local benchmark, 3-layer splits, 23/24 tests) is sufficient.

**Tandem pattern:** Local qwen-64k drafts the design → frontier mid-tier reviews for ambiguity, missing acceptance criteria, architectural soundness → human approves.

#### Agent D2: Design Documenter

**Responsibility:** README, usage guides, vocabulary definitions, docstrings. Produces the documentation that accompanies the design package.

**Model:** Tier 0 local — qwen3-coder-30b.

**Why:** qwen3-coder is the only model that writes complete READMEs (3/3 doc score). Documentation is high-volume, low-reasoning work. No need for a frontier model here.

---

### Verification Contract Phase

Currently: `hermoso-verification-author` (one agent authors the entire hidden contract)

#### Agent V1: Contract Author

**Responsibility:** Gherkin scenarios, rubric criteria, judgment definitions, traceability from requirements to judgments. Designs the verification contract structure.

**Model:** Tier 2 (mid frontier).

**Why:** The verification contract is the quality gate. If it's wrong, everything downstream is meaningless. This requires precision, completeness, and the ability to think about edge cases that the design didn't specify. A mid-tier frontier model is worth the cost here — mistakes in the contract can't be caught later.

**Tandem pattern:** Frontier mid-tier authors the contract → human reviews before approval. No local model in this loop — the contract is too important.

#### Agent V2: Fixture/Probe Builder

**Responsibility:** Deterministic test fixtures, probe scripts, Gherkin step definitions. Writes the code that the contract references.

**Model:** Tier 0 local — qwen3-coder-30b with TDD prompts.

**Why:** Fixtures are code generation with clear specs (the contract defines exactly what to test). qwen3-coder is fast and accurate on well-specified code. A frontier code reviewer (Agent R1) checks the fixtures against the contract.

---

### Construction Phase

Currently: `hermoso-construction` + `kanban-worker` (one worker type handles all construction)

#### Agent C1: Work-Graph Orchestrator

**Responsibility:** Decompose the approved design into work items, topological ordering, dependency analysis. Produces the work graph that Hermoso validates and dispatches.

**Model:** Tier 2 (mid frontier) for complex graphs. Tier 1 (cheap frontier) for simple graphs.

**Why:** Graph correctness is critical — wrong dependencies mean blocked workers or integration failures. This is pure reasoning, no code generation. A frontier model handles this in one pass with a small prompt (design summary + model query results).

**No local model here:** The work graph determines dispatch order. Getting it wrong wastes worker time. The cost of one frontier call is trivial compared to wasted local inference.

#### Agent C2: New-Code Writer

**Responsibility:** Greenfield implementation — new packages, new interfaces, new files. Architecture-heavy code where structure matters.

**Model:** Tier 0 local — qwen-64k (best architecture, 23/24 tests pass). Or qwen-3.8-27b for correctness-critical modules. Or Tier 1 (cheap frontier) for complex modules where architecture mistakes are costly.

**Why:** qwen-64k writes the best-structured code locally. With TDD prompts (prevents over-engineering) and a frontier code reviewer catching signature changes, it's the best local choice for most modules. **qwen-3.8-27b is the correctness upgrade**: 35/35 tests pass vs qwen-64k's 23/24, and it is the only model that avoids the signature-change trap. It's ~3× slower than qwen-64k and 39× slower than qwen3-coder, but construction workers run autonomously in batch — no human is waiting — so for the correctness-critical modules (domain logic, storage, cross-package interfaces) the extra latency buys a lower remediation rate. For modules with complex interfaces, a cheap frontier model may still produce better code in one pass.

**Tandem pattern:** Local qwen-64k (or qwen-3.8-27b for correctness-critical modules) writes the code → frontier cheap-tier reviews the diff (Agent R1) → if issues found, local qwen3-coder remediates (Agent C3).

#### Agent C3: Code Editor

**Responsibility:** Surgical edits to existing code — bug fixes, small modifications, feature additions to existing functions. Speed matters, accuracy matters, architecture less so.

**Model:** Tier 0 local — qwen3-coder-30b.

**Why:** qwen3-coder is 10-18x faster than other local models on edits, equally accurate on tight specs, and produces purely additive diffs (never rewrites existing code). For editing tasks, it's the clear choice.

**Tandem pattern:** Local qwen3-coder makes the edit → frontier cheap-tier reviews the diff → if issues, qwen3-coder remediates in 5s.

#### Agent C4: Test Writer

**Responsibility:** TDD test authoring, test harness design, test fixtures for worker cards. Writes tests that fail first, then code that passes.

**Model:** Tier 0 local — qwen3-coder-30b with TDD prompts.

**Why:** qwen3-coder is fastest (5-10s per test) and TDD framing keeps it focused. Tests are well-specified by definition (the spec says what to test). No frontier model needed for generation.

**Tandem pattern:** Local qwen3-coder writes tests → frontier cheap-tier checks test adequacy (coverage, edge cases) → if gaps, qwen3-coder adds tests.

#### Agent C5: Integration Manager

**Responsibility:** Merge branches, resolve conflicts, run baseline checks after fan-in. Handles the integration work item when multiple leaves need merging.

**Model:** Tier 1 (cheap frontier) for conflict resolution. Human for complex conflicts.

**Why:** Merge conflicts require understanding two code paths and choosing the right resolution. Local models haven't been tested on this, and it's low-frequency enough that a frontier model is worth the cost. For simple merges (no conflicts), the Go CLI handles it deterministically.

---

### Verification Phase

Currently: `hermoso-verification` (one agent runs mechanical checks and makes qualitative judgments)

#### Agent E1: Mechanical Runner

**Responsibility:** Execute Hermoso verification CLI, record evidence, parse Phase 1 report. Deterministic orchestration — no judgment.

**Model:** None. This is the Hermoso Go CLI. It runs commands, records output, hashes artifacts. No LLM needed.

**Why:** Mechanical verification is deterministic by design. The whole point of Hermoso's verification contract is that Phase 1 is machine-runnable. Adding an LLM here adds noise.

#### Agent E2: Qualitative Judge

**Responsibility:** Rubric assessment — read evidence, apply rubric criteria, produce pass/fail with reasoning. Surface resolution (matching planned surfaces to implemented model nodes).

**Model:** Tier 2 (mid frontier). Tier 0 local (qwen-3.8-27b) as the correctness-optimized fallback; gemma as the fast fallback.

**Why:** The qualitative judge is the final quality gate before release. It needs to read code carefully, reason about whether it meets rubric criteria, and produce a defensible judgment. qwen-3.8-27b is the strongest *local* judge: its thinking mode is exactly what careful rubric judgment wants (it is the only local model to reason past the signature-change trap, and it produces the most correct code). It is slow and verbose, but the judge runs in batch during verification — no human is waiting — so that latency is acceptable. A mid-tier frontier model remains primary for the highest-stakes features; gemma remains for when a fast, concise local judgment is preferred.

**Tandem pattern:** For high-stakes features, run both: local qwen-3.8-27b produces a judgment, frontier mid-tier produces a judgment. If they agree, proceed. If they disagree, escalate to human. This costs one frontier call but catches judge errors. (gemma can substitute for qwen-3.8-27b when wall-clock matters more than correctness.)

#### Agent E3: Remediation Author

**Responsibility:** Write targeted fix specs for failed verification outcomes. Produces the SkillRemediation spec with specific needs and a work graph for the remediation round.

**Model:** Tier 1 (cheap frontier) or Tier 0 local (qwen3-coder with TDD prompts).

**Why:** Remediation specs need to be specific (not "fix the implementation") and targeted. A cheap frontier model can read the failure evidence and write a precise fix spec. For simple failures (compile errors, missing test cases), qwen3-coder is faster and sufficient.

**Tandem pattern:** Frontier cheap-tier reads the failure and writes the remediation spec → local qwen3-coder executes the fix → frontier cheap-tier verifies the fix resolves the original failure.

---

### Cross-Cutting (New)

These don't exist in the current Hermoso lifecycle. They're quality gates that run across all phases.

#### Agent R1: Code Reviewer

**Responsibility:** Pre-commit review on every code change. Checks for the 6 universal guardrail violations: (1) Maven instead of Gradle, (2) signature changes, (3) missing TDD framing, (4) import hygiene, (5) full-file rewrites, (6) unnecessary thinking mode. Also checks for race conditions and over-engineering.

**Model:** Tier 1 (cheap frontier).

**Why:** This is the most valuable tandem agent. All 6 guardrail violations are detectable from a diff — a cheap frontier model can check them in one pass. This catches the signature-change trap (run-007), the over-engineering regression (run-006), and the import failures (run-002) before they reach verification. The cost is one cheap API call per commit — negligible.

**Runs after:** Every Agent C2, C3, C4 output. Reviews the diff, not the full file.

**Output:** Pass/fail per guardrail. If fail, routes back to the generating agent with specific feedback.

#### Agent R2: Context Monitor

**Responsibility:** Watch context window usage. Trigger file-only patches (not full-file rewrites) when context exceeds 40K. Alert when approaching model context limits.

**Model:** None. Deterministic logic — check token count, compare to limit, trigger action. Can be a Hermes cronjob or inline in the skill.

**Why:** The benchmark showed that 40K+ context works on all models, but the bottleneck is prompt design (full-file rewrite). A deterministic monitor that switches to function-only patches at 40K prevents the timeout issues seen in run-008.

---

## 5. Agent Summary Table

| Agent | Phase | Responsibility | Tier | Model | Runs on |
|-------|-------|---------------|------|-------|---------|
| D1 | Design | Feature design architect | 2 (or 0 local) | Frontier mid / qwen-64k | OpenRouter / local |
| D2 | Design | Documentation writer | 0 | qwen3-coder-30b | Local |
| V1 | Verification contract | Contract author | 2 | Frontier mid | OpenRouter |
| V2 | Verification contract | Fixture/probe builder | 0 | qwen3-coder-30b | Local |
| C1 | Construction | Work-graph orchestrator | 2 (or 1) | Frontier mid/cheap | OpenRouter |
| C2 | Construction | New-code writer | 0 (or 1) | qwen-64k / qwen-3.8-27b (correctness-critical) / frontier cheap | Local / OpenRouter |
| C3 | Construction | Code editor | 0 | qwen3-coder-30b | Local |
| C4 | Construction | Test writer | 0 | qwen3-coder-30b | Local |
| C5 | Construction | Integration manager | 1 | Frontier cheap | OpenRouter |
| E1 | Verification | Mechanical runner | — | Go CLI (no LLM) | Hermoso binary |
| E2 | Verification | Qualitative judge | 2 (or 0) | Frontier mid / qwen-3.8-27b (correctness) / gemma (fast) | OpenRouter / local |
| E3 | Verification | Remediation author | 1 (or 0) | Frontier cheap / qwen3-coder | OpenRouter / local |
| R1 | Cross-cutting | Code reviewer | 1 | Frontier cheap | OpenRouter |
| R2 | Cross-cutting | Context monitor | — | Deterministic logic | — |

**Token distribution estimate:** ~85% local, ~15% frontier. Frontier calls are small (diffs, reviews, judgments — not full file contents).

---

## 6. Tandem Patterns in Detail

### Pattern A: Code Generation Cycle

```
1. Agent C2 or C3 generates code (local, 5-130s)
2. Agent R1 reviews diff (frontier cheap, ~2-5s, ~$0.001-0.01)
3a. If PASS: proceed to commit
3b. If FAIL: Agent C3 remediates with R1 feedback (local, 5s) → back to step 2
4. Max 3 remediation rounds before human escalation
```

**Cost per cycle:** ~$0.01-0.05 in frontier API calls. Local inference is free.
**Time per cycle:** 10-140s local + 5s frontier review = 15-145s total.

### Pattern B: Test Generation Cycle

```
1. Agent C4 writes tests (local qwen3-coder, 5-10s)
2. Agent R1 checks test adequacy — coverage, edge cases (frontier cheap, ~2-5s)
3a. If PASS: proceed
3b. If FAIL: Agent C4 adds missing tests → back to step 2
```

### Pattern C: Design Review Cycle

```
1. Agent D1 drafts feature design (local qwen-64k for simple, frontier mid for complex)
2. Frontier mid-tier reviews for ambiguity, missing criteria, architectural soundness
3. Human reviews and approves
4. Agent V1 authors verification contract (frontier mid, independent of D1)
5. Human reviews contract before approval
```

### Pattern D: Remediation Cycle

```
1. Agent E2 judges verification outcome (frontier mid, ~5-10s)
2. If FAIL: Agent E3 authors remediation spec (frontier cheap, ~5s)
3. Agent C3 executes fix (local qwen3-coder, 5s with TDD prompt)
4. Agent R1 reviews the fix diff (frontier cheap, ~2-5s)
5. Agent E1 re-runs mechanical verification (Go CLI, deterministic)
6. If second fail: blocked, human escalation
```

### Pattern E: Dual-Judge (High-Stakes)

```
1. Agent E2-local (gemma) produces judgment
2. Agent E2-frontier (mid-tier) produces judgment
3. If agree: proceed
4. If disagree: human reviews both judgments and decides
```

**When to use:** Features touching core domain logic, security-sensitive code, or cross-component interfaces. Cost: one extra frontier call. Benefit: catches judge errors.

---

## 7. Cost Model

### Per-Feature Cycle Estimate

| Step | Model | Input tokens | Output tokens | Est. cost |
|------|-------|-------------|--------------|-----------|
| Design review (D1 tandem) | Frontier mid | ~2K | ~1K | ~$0.02-0.05 |
| Work-graph (C1) | Frontier mid | ~1K | ~1K | ~$0.01-0.03 |
| Code reviews (R1, ~5 calls) | Frontier cheap | ~5K total | ~2K total | ~$0.01-0.02 |
| Contract author (V1) | Frontier mid | ~3K | ~2K | ~$0.03-0.08 |
| Verification judge (E2) | Frontier mid | ~5K | ~1K | ~$0.02-0.08 |
| Remediation (E3, if needed) | Frontier cheap | ~2K | ~1K | ~$0.005-0.01 |
| **Total frontier cost** | | ~18K | ~8K | **~$0.10-0.27** |

Local inference: $0 (already running, solar-powered).
Wall-clock: 15-30 minutes per feature (local generation + frontier reviews, sequential).

**Comparison:** Running everything on a frontier model would cost $1-5 per feature and be faster, but the user's strategy is cost-optimized with acceptable wall-clock. The tandem approach costs ~$0.15/feature vs ~$3/feature for all-frontier — a 20x cost reduction at the expense of ~2-3x wall-clock.

---

## 8. What Changes from FINAL-SYNTHESIS.md

| Hermoso role | Old (local-only) | New (tiered) | Change |
|-------------|-----------------|-------------|--------|
| Design | qwen3-coder (speed + docs) | D1: frontier mid (complex) / qwen-64k (simple) + D2: qwen3-coder (docs) | Split design architect from documenter. Frontier reviews design. |
| Construction (new code) | qwen-64k (best architecture) | C2: qwen-64k (local) / qwen-3.8-27b (correctness-critical) + R1: frontier cheap review | Added qwen-3.8-27b for correctness-critical modules; added automated code review. |
| Construction (edits) | qwen3-coder (speed) | C3: qwen3-coder (local) + R1: frontier cheap review | Same generation model, added frontier review. |
| Verification (judge) | gemma (most reliable) | E2: frontier mid (primary) / qwen-3.8-27b (correctness fallback) / gemma (fast fallback). Optional dual-judge. | Added qwen-3.8-27b as the correctness-optimized local judge; frontier stays primary. |
| Remediation | qwen3-coder (fast fixes) | E3: frontier cheap (spec author) + C3: qwen3-coder (fix execution) | Split spec authoring from fix execution. |
| Release (future) | qwen3-coder | D2: qwen3-coder (docs) + E2: frontier mid (final approval) | Added frontier final approval gate. |
| — (new) | — | C1: frontier mid/cheap (work-graph) | New agent for graph orchestration. |
| — (new) | — | R1: frontier cheap (code review) | New cross-cutting review agent. |
| — (new) | — | R2: deterministic (context monitor) | New cross-cutting monitor. |

---

## 9. Universal Prompt Guardrails (Unchanged)

These apply to all models, all tiers, all phases:

1. **"Use Gradle, not Maven"** for Java tasks
2. **"Do not modify existing function signatures. Add new methods instead."**
3. **Include the failing test when available** (TDD framing)
4. **"Ensure all imports are used and all used imports are present"**
5. **Forbid full-file rewrites on large files** — ask for the changed function only
6. **Disable thinking mode on clear specs** — reasoning adds latency with no quality gain *for the Qwen 3.6 models that can toggle it*. **Exception: qwen-3.8-27b is a fixed thinking model** — its reasoning is always on and, for stateful/correctness-critical tasks (multi-turn, rubric judgment, correctness-critical generation), that reasoning is the point. Reserve it for those batch roles; never route it to an interactive edit where the latency is pure overhead.

The frontier code reviewer (R1) checks all 6 on every diff.

---

## 10. What Frontier Models Still Can't Fix (Human Escalation Points)

Even with tandem patterns, some issues require human judgment:

1. **Ambiguous requirements** — if the design spec is genuinely ambiguous, no model can resolve it. Human must clarify.
2. **Cross-component architecture decisions** — when a feature touches multiple packages and the right boundary is a judgment call. Tier 3 (premium) can advise, but human decides.
3. **Contract design trade-offs** — what to test vs. what to trust. The verification contract encodes judgment about risk. Human should review V1's output.
4. **Second-fail remediation** — if the first remediation round fails, the issue is likely a design or contract problem, not a coding error. Human escalation, not a third round.

---

## 11. Migration Path (Advice Only — No Hermoso Changes This Session)

1. **Phase 1:** Configure OpenRouter as a Hermes provider (already done — user has subscription)
2. **Phase 2:** Define model aliases in Hermes config for each tier (e.g., `frontier-cheap`, `frontier-mid`, `frontier-premium`)
3. **Phase 3:** Split Hermoso skills at the boundaries identified above:
   - `hermoso-design` → `hermoso-design-architect` (D1) + `hermoso-design-docs` (D2)
   - `hermoso-verification-author` → `hermoso-contract-author` (V1) + `hermoso-fixture-builder` (V2)
   - `hermoso-construction` → `hermoso-workgraph` (C1) + `hermoso-worker-newcode` (C2) + `hermoso-worker-edit` (C3) + `hermoso-worker-test` (C4) + `hermoso-integration` (C5)
   - `hermoso-verification` → `hermoso-verification-runner` (E1, no LLM) + `hermoso-verification-judge` (E2) + `hermoso-remediation-author` (E3)
   - New: `hermoso-code-reviewer` (R1) + `hermoso-context-monitor` (R2)
4. **Phase 4:** Update `profiles/default.yaml` to bind each sub-agent to its tier-appropriate model
5. **Phase 5:** Test tandem patterns on a single Hermoso feature cycle
6. **Phase 6:** Measure cost per feature, adjust tier assignments based on actual frontier model performance

---

## 12. Open Questions

1. **Which specific OpenRouter models for each tier?** The strategy defines tiers by capability/cost. The user should pick specific models from their subscription. Candidates:
   - Tier 1 (cheap): claude-haiku-4, gpt-4o-mini, gemini-2-flash, deepseek-chat
   - Tier 2 (mid): claude-sonnet-4, gpt-4.1, deepseek-v4
   - Tier 3 (premium): claude-opus-4, o3, gpt-4.1-pro
2. **Code reviewer (R1) trigger:** Automatic after every commit, or only at pre-PR gate? Automatic is recommended — the cost is negligible (~$0.01/call) and it catches issues early.
3. **Dual-judge (Pattern E) default or opt-in?** Recommended opt-in for high-stakes features only. Default to single frontier judge.
4. **Context monitor (R2) implementation:** Hermes cronjob or inline skill logic? Inline is simpler — the skill checks token count before each call.
5. **Should the work-graph orchestrator (C1) ever run on local?** For simple features with 1-2 work items, local qwen-64k might suffice. Threshold: >3 work items → frontier.
