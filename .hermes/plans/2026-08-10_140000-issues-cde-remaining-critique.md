# Issues C, D, E: Remaining Verification Loop Critique — Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Resolve the remaining three critique issues: remove the Gherkin placeholder scan (E), remove the redundant identity ritual from skills (D), and move planned surface resolution from Go's fuzzy title match to skill-authored resolutions (C).

**Architecture:** Three independent fixes, ordered by complexity:
- Issue E: Delete 5 lines from `parseGherkin`, add checklist item to authoring skill
- Issue D: Update 5 skill SKILL.md files + skills test to remove echo/compare ritual (binary already enforces identity)
- Issue C: Two-phase surface resolution reusing `AwaitingJudgment` — `verification run` marks planned surfaces as pending, add `verification resolve` command for skill-authored resolutions

**Branch:** `bdd-contracts-and-verification` (worktree at `/home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification`)

## Issue E: Remove Gherkin placeholder scan (trivial)

### Task E1: Remove placeholder scan from parseGherkin

Location: `internal/verification/contract.go`, lines 252-258.

Remove the `upper` variable and the `for` loop that scans for TODO/TBD/NEEDS CLARIFICATION/[PLACEHOLDER].

### Task E2: Add checklist item to hermoso-verification-author SKILL.md

Add a content-quality checklist item about placeholders in the "Gherkin tags" section.

## Issue D: Remove identity ritual from skills (moderate)

### Task D1: Update skills test

`TestSkillsRequireExplicitContextChecks` in `skills/skills_test.go` currently requires `"absolute workspace"`, `"block"`, `"mismatch"` in every skill. Update to require `"never infer"`, `"hermoso context"`, and `"explicit"` instead.

### Task D2: Update all 5 skills

Replace echo/compare/block ritual with: "Pass explicit context tuple to every command. The binary validates against persisted state and rejects mismatches. Never infer identity from cwd or conversation."

Files: `skills/hermoso/SKILL.md`, `skills/hermoso-design/SKILL.md`, `skills/hermoso-construction/SKILL.md`, `skills/hermoso-verification/SKILL.md`, `skills/hermoso-verification-author/SKILL.md`.

## Issue C: Two-phase surface resolution (complex)

### Task C1: Change resolveCandidateSurfaces

Remove `strings.EqualFold(node.Title, surface.Title)` fuzzy match. Keep `surface_id` attribute match (cooperative tagging — deterministic). Mark unmatched planned surfaces as `"pending"` instead of `"missing"`. Update verdict aggregation: pending surfaces cause `VerificationPending`.

### Task C2: Add PutSurfaceResolutions method

New method accepts `[]SurfaceResolution`, validates all pending surfaces are resolved, applies the invariant (unresolved surfaces fail linked judgments), re-aggregates verdict. If verdict is still pending (rubric judgments also needed), stays in `AwaitingJudgment`. Also update `JudgeVerification` to handle `VerificationPending` case by staying in `AwaitingJudgment`.

### Task C3: Add verification resolve CLI subcommand

`hermoso verification resolve <context> <resolutions.json>` reads JSON and calls `PutSurfaceResolutions`.

### Task C4: Update hermoso-verification SKILL.md

Document the surface resolution flow: query model snapshot, resolve pending surfaces, submit via `verification resolve`.

## Finalize

### Task F1: Mark C, D, E resolved in critique doc

### Task F2: Full test suite verification
