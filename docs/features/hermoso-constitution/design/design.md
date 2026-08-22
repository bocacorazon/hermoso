# Hermoso Constitution — Project Governance with Hard-Gate Enforcement

## Feature summary

This feature adds a constitution governance system to Hermoso. Every Hermoso-managed project gets a versioned constitution at `docs/constitution.md` that formalizes its governing principles. A new `hermoso-constitution` skill interactively creates and amends the constitution. The `hermoso-design` and `hermoso-construction` skills enforce the constitution as a hard gate — blocking on violations with a formal escape-hatch override mechanism.

## Complexity assessment

**Standard.** The feature touches multiple skills (new constitution skill + modifications to hermoso-design, hermoso-construction, skills_test.go, and default.yaml) and introduces a new domain concept (constitution as governance artifact). It's within one repo, no new Go CLI commands, no cross-package changes.

## Clarifications

1. **Scope:** Constitution skill + integration with design/construction skills. Not a Go CLI command.
2. **Storage:** `docs/constitution.md` (committed markdown). No `.hermoso/` state changes.
3. **Applicability:** General capability for any Hermoso-managed project. Hermoso repo is the first dogfood user.
4. **Enforcement:** Hard gate at design and construction, with a formal escape hatch.
5. **Init flow:** Post-init — the skill creates the starter constitution after `hermoso init`.

## Alternatives considered

### Approach A: Pure skill artifact (chosen)
Constitution at `docs/constitution.md`, skill-managed, no Go CLI. Enforcement is skill-level judgment.

### Approach B: Go CLI command
New `hermoso constitution init|put|show|check` commands. Go validates and enforces.
**Rejected:** "Does this design violate a principle?" is judgment, not deterministic. Violates the skill/Go boundary.

### Approach C: Hybrid — skill creates, Go stores hash
Skill creates the constitution, one new Go command computes/stores the hash.
**Rejected for v1:** Minimal Go benefit. Can add later if freshness verification becomes important.

## Design overview

### Requirements

- **req-constitution-skill** (must): New skill for interactive constitution creation/amendment
- **req-constitution-storage** (must): Constitution stored as committed markdown at `docs/constitution.md`
- **req-constitution-post-init** (must): Post-init creation flow; design blocks if no constitution
- **req-design-gate** (must): Design-phase hard gate reading the constitution before `design put`
- **req-construction-gate** (must): Construction-phase hard gate before dispatch
- **req-escape-hatch** (must): Formal override via design's decisions array with rationale + approval
- **req-general-capability** (should): Works for any Hermoso-managed project
- **req-skill-invariants** (must): All 5 skill test invariants pass

### Surfaces

- `skills/hermoso-constitution/SKILL.md` (new) — the constitution skill
- `skills/hermoso-design/SKILL.md` (modified) — constitution gate at entry
- `skills/hermoso-construction/SKILL.md` (modified) — constitution gate before dispatch
- `profiles/default.yaml` (modified) — skill binding
- `skills/skills_test.go` (modified) — add to skillNames
- `docs/constitution.md` (runtime artifact) — per-project, created by the skill

### Key decisions

1. **`docs/constitution.md`, not `.hermoso/`** — committed, visible, diffable
2. **No Go CLI for v1** — judgment is a skill task, not a Go task
3. **Required for design, post-init creation** — avoids Go changes, enforces at design time
4. **Escape hatch via decisions array** — no schema changes needed
5. **Controller-phase skill** — project-level governance, not phase-specific

## Verification approach

The verification contract uses:
- **exit_code oracle** for the Go skill test suite (`go test ./skills/ -v -run TestSkill`)
- **stdout_regex oracle** for the profile binding check
- **rubric oracle** for skill content quality checks (constitution skill structure, design gate presence, construction gate presence, escape hatch mechanism, general capability)

Probes are Python scripts that read the skill files and check for required content patterns. The rubric judgments evaluate whether the content meets the quality criteria.

## Open questions

None — all questions were resolved during clarification.
