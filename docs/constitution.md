# Hermoso Constitution

Guiding principles and quality gates for Hermoso — a Go control plane for Hermes-driven feature development.

## Core Principles

### I. Skill/Go Boundary (Non-Negotiable)

- Skills own ALL reasoning — design, classification, state mutation decisions, and verification.
- The Go CLI is a deterministic shell that exposes state and contracts via JSON. It MUST NOT do fuzzy matching, natural-language classification, or break ties.
- Skills mutate `.hermoso/` state exclusively through CLI commands — never by direct file writes. The CLI is the single entry point for state transitions.
- **Rationale:** The boundary keeps reasoning where it belongs (in the agent, which can adapt) and keeps the CLI deterministic (predictable, testable, auditable). Mixing the two produces an untestable mess where neither layer can be trusted.

### II. Verification Gate (Non-Negotiable)

- Every feature MUST pass formal verification before release.
- Verification contracts are written at design time and executed at verification time.
- A formal escape-hatch override — declared in the design's `decisions` array — is required to bypass verification. Waivers without documentation are forbidden.
- **Rationale:** Verification proves the feature works as designed. Without a hard gate, verification becomes optional ceremony and defects reach production. The escape hatch exists for genuine edge cases (e.g., external dependency unavailable) but requires a paper trail.

### III. Implementation Language (Non-Negotiable)

- New components MUST be written in Go.
- Language fluency order: Go > Python > JS UI. When a component falls between tiers, Go wins.
- **Rationale:** Single-language codebase reduces context-switching cost, keeps the binary static, and avoids the dependency sprawl of multi-language projects. Hermoso is a Go CLI — new code in other languages creates orphaned maintenance burdens.

### IV. Test-Driven Development (Mandatory)

- RED-GREEN-REFACTOR: tests MUST be written before implementation.
- Every feature ships with passing tests. Untested features are incomplete.
- Tests run as part of every build. Broken tests block release.
- **Rationale:** TDD catches design flaws before they become implementation. Tests written after the fact tend to confirm existing behavior rather than define correct behavior. A test-first discipline ensures every feature has a verifiable contract.

### V. Minimal Dependencies (Mandatory)

- Hermoso ships as a static binary with minimal external dependencies.
- Every new dependency MUST be explicitly justified in the design document. Prefer Go stdlib.
- Dependencies that bloat the binary, introduce build complexity, or reduce portability are rejected.
- **Rationale:** Dependencies are liabilities — each one is a maintenance commitment, a security surface, and a potential upgrade headache. A Go CLI that can do its job with stdlib alone is indefinitely portable and trivially deployable.

### VI. Design Process (Mandatory)

- Features follow an interactive, multi-phase workflow:
  1. **Classify** — determine complexity (trivial/standard/complex)
  2. **Clarify** — resolve ambiguities with the operator
  3. **Alternatives** — present 2-3 design choices for standard/complex features
  4. **Author** — write the design document
  5. **Verify** — formal read-only verification against the contract
  6. **Construct** — intentional implementation, no code before approved design
  7. **Release** — merge to main, publish artifacts
- Construction is intentional, not speculative. No implementation code before the design is approved.
- Verification is read-only: verifiers observe and report, never mutate code or state.
- **Rationale:** Speculative coding wastes time on the wrong solution. Multi-phase design catches bad decisions before they become code. Read-only verification ensures the verifier cannot "fix" what it's supposed to judge.

### VII. Audit Trail (Recommended)

- Hermoso SHOULD log errors and key state transitions with timestamps.
- Full reproducibility from inputs is not required. Basic logging suffices.
- **Rationale:** A personal tool doesn't need the rigor of a build system. But when something breaks, the operator needs enough context to understand what happened. Error logs and state transition records provide that without the overhead of full determinism.

### VIII. No Secrets (Non-Negotiable)

- API keys, tokens, passwords, and credentials MUST NOT appear in committed code, logs, summary output, or `.hermoso/` state files.
- Secrets live in environment variables or dedicated credential stores only.
- **Rationale:** Secrets in version control are permanent, discoverable, and impossible to truly revoke. Hermoso operates on repositories — it MUST NOT become a vector for credential leaks.

## Governance

- **Amendment procedure:** Principles are added, modified, or removed by running the `hermoso-constitution` skill. Amendments update the version, record changes in a sync impact report, and are committed to the repository.
- **Versioning:** Semantic versioning — MAJOR for incompatible principle removals or redefinitions, MINOR for new principles or material expansions, PATCH for clarifications and wording fixes.
- **Compliance review:** Feature designs are checked against the constitution at design time (hard gate) and construction time (hard gate). Violations require a formal escape-hatch override in the design's `decisions` array.
- **Ratification date:** 2026-09-20
- **Last amended:** 2026-09-20
- **Constitution version:** 1.0.0