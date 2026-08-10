# Issue B: Skill-Authored Remediation Spec — Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Split remediation into two phases — Go transitions to `StatusAwaitingRemediation` on verification failure, a skill authors the remediation needs + work graph, Go validates and persists — mirroring the Issue A pattern.

**Architecture:** When verification fails (attempt 1), Go stops creating the remediation graph itself. Instead it transitions the run to `{PhaseConstruction, StatusAwaitingRemediation}`. The skill reads the failed report (already returned by `verification run` / `verification judge`), authors a `SkillRemediation` (needs + work graph with targeted prompts), and submits it via `verification remediate <context> <spec-path>`. Go validates structure, wraps it in a `ConstructionState` with deterministic fields, and transitions to `{PhaseConstruction, StatusPending}`.

**Tech Stack:** Go 1.26+, Tree-sitter (CGO), standard library `encoding/json`

**Branch:** `bdd-contracts-and-verification` (worktree at `/home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification`)

---

## Context

### Current behavior (the problem)

`remediationRound` (verification.go:584-654) is called synchronously inside `persistVerificationAttempt` (line 280) and `JudgeVerification` (line 817) when verification fails on attempt 1. It:

1. Iterates failed outcomes
2. Creates `RemediationNeed` entries with `Expected` (acceptance criteria text) and `Actual` (outcome summary)
3. Creates `WorkItem` entries with hardcoded prompt: `"Correct the implementation to satisfy the referenced visible requirements and acceptance criteria. Observed summary: " + outcome.Summary`
4. Sets `Producer: {Skill: "hermoso-verification", Runtime: "deterministic"}` — a category error
5. Wraps in `ConstructionState`, appends to `run.ConstructionRounds`, transitions to `{PhaseConstruction, StatusPending}`

### Desired behavior

**Phase 1** (Go-owned, in `persistVerificationAttempt` / `JudgeVerification`):
- Enforce two-attempt limit (already done)
- Transition to `{PhaseConstruction, StatusAwaitingRemediation}` — no remediation round created
- Failed report + evidence already returned to caller

**Phase 2** (Skill-owned, via `PutRemediation`):
- Skill reads failed report, authors `SkillRemediation` (needs + work graph)
- Go validates: run is `StatusAwaitingRemediation`, context matches, graph passes `Validate()`, needs non-empty
- Go creates `ConstructionState` with deterministic fields (Number, Kind, SourceHash, Hash, RemediationSpec wrapper) + skill-authored fields (Needs, Graph)
- Go transitions to `{PhaseConstruction, StatusPending}`

### Key types

```go
// internal/domain/verification_report.go — new type alongside SkillJudgment
type SkillRemediation struct {
    Needs []RemediationNeed `json:"needs"`
    Graph WorkGraph         `json:"graph"`
}
```

Go sets on the `ConstructionState`:
- `Number`: `len(run.ConstructionRounds) + 1`
- `Kind`: `"remediation"`
- `SourceHash`: last verification attempt's `ReportHash`
- `RemediationSpec.SchemaVersion`, `.Context`, `.FailedReportHash`, `.CreatedAt`
- `Graph.Hash`

Skill authors:
- `RemediationSpec.Needs` (what went wrong: Expected, Actual, surface/requirement/criterion IDs)
- `WorkGraph` (how to fix: items with targeted prompts, workers, dependencies, Producer)

### Lifecycle transitions

New status: `StatusAwaitingRemediation RunStatus = "awaiting_remediation"` in `PhaseConstruction`.

New legal transitions:
- `{PhaseConstruction, StatusAwaitingVerification}` → `{PhaseConstruction, StatusAwaitingRemediation}` (verification fail, attempt 1)
- `{PhaseVerification, StatusAwaitingJudgment}` → `{PhaseConstruction, StatusAwaitingRemediation}` (judge fail, attempt 1)
- `{PhaseConstruction, StatusAwaitingRemediation}` → `{PhaseConstruction, StatusPending}` (skill submits remediation)
- `{PhaseConstruction, StatusAwaitingRemediation}` → `{PhaseConstruction, StatusBlocked}`
- `{PhaseConstruction, StatusAwaitingRemediation}` → `{PhaseConstruction, StatusCancelled}`

---

## Files

### Create
- `internal/domain/lifecycle_remediation_test.go` — tests for `StatusAwaitingRemediation`
- `internal/workflow/put_remediation_test.go` — tests for `PutRemediation`
- `internal/workflow/remediation_integration_test.go` — integration test for full two-phase flow
- `internal/app/remediate_cmd_test.go` — CLI test

### Modify
- `internal/domain/lifecycle.go` — add `StatusAwaitingRemediation`, update `statusAllowed`, add transitions
- `internal/domain/verification_report.go` — add `SkillRemediation` type
- `internal/workflow/verification.go` — change fail paths, add `PutRemediation`, remove `remediationRound`
- `internal/workflow/service.go` — no changes needed (service struct unchanged)
- `internal/workflow/service_test.go` — update `TestVerificationFailureCreatesOneRemediationRound`
- `internal/workflow/judge_verification_test.go` — update fail test expectations
- `internal/workflow/rubric_integration_test.go` — update fail test expectations
- `internal/app/app.go` — add `remediate` subcommand to `runVerificationContract`
- `skills/hermoso-verification/SKILL.md` — document two-phase remediation flow
- `docs/verification-loop-critique.md` — mark Issue B as resolved

---

## Tasks

### Task 1: Add StatusAwaitingRemediation to lifecycle

**Objective:** Add the new run status and its legal transitions.

**Files:**
- Modify: `internal/domain/lifecycle.go`
- Create: `internal/domain/lifecycle_remediation_test.go`

**Step 1: Write failing test**

Create `internal/domain/lifecycle_remediation_test.go`:

```go
package domain

import "testing"

func TestStatusAwaitingRemediationAllowed(t *testing.T) {
	t.Parallel()
	if !statusAllowed(PhaseConstruction, StatusAwaitingRemediation) {
		t.Errorf("statusAllowed should permit %s/%s", PhaseConstruction, StatusAwaitingRemediation)
	}
}

func TestAwaitingRemediationTransitions(t *testing.T) {
	t.Parallel()
	src := lifecycleState{PhaseConstruction, StatusAwaitingRemediation}
	destinations := []lifecycleState{
		{PhaseConstruction, StatusPending},
		{PhaseConstruction, StatusBlocked},
		{PhaseConstruction, StatusCancelled},
	}
	for _, dst := range destinations {
		if _, ok := legalTransitions[src][dst]; !ok {
			t.Errorf("missing transition %s/%s → %s/%s", src.phase, src.status, dst.phase, dst.status)
		}
	}
}

func TestAwaitingRemediationFromVerificationFail(t *testing.T) {
	t.Parallel()
	cases := []struct{ from lifecycleState; to lifecycleState }{
		{PhaseConstruction, StatusAwaitingVerification}: {PhaseConstruction, StatusAwaitingRemediation},
		{PhaseVerification, StatusAwaitingJudgment}:     {PhaseConstruction, StatusAwaitingRemediation},
	}
	for _, c := range cases {
		if _, ok := legalTransitions[c.from][c.to]; !ok {
			t.Errorf("missing transition %s/%s → %s/%s", c.from.phase, c.from.status, c.to.phase, c.to.status)
		}
	}
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestStatusAwaitingRemediation -v`
Expected: FAIL — `statusAllowed should permit construction/awaiting_remediation`

**Step 3: Implement**

In `internal/domain/lifecycle.go`:

1. Add constant after line 31 (`StatusAwaitingJudgment`):
```go
StatusAwaitingRemediation RunStatus = "awaiting_remediation"
```

2. In `statusAllowed` (line 472-488), add to `PhaseConstruction` case:
```go
return status == StatusPending || status == StatusInProgress || status == StatusCompleted || status == StatusAwaitingVerification || status == StatusAwaitingRemediation
```

3. Add transition entries in `legalTransitions` map:
   - In `{PhaseConstruction, StatusAwaitingVerification}` entry, add:
     `{PhaseConstruction, StatusAwaitingRemediation}: {},`
   - In `{PhaseVerification, StatusAwaitingJudgment}` entry, add:
     `{PhaseConstruction, StatusAwaitingRemediation}: {},`
   - Add new entry:
     `{PhaseConstruction, StatusAwaitingRemediation}: { {PhaseConstruction, StatusPending}: {}, {PhaseConstruction, StatusBlocked}: {}, {PhaseConstruction, StatusCancelled}: {} },`

**Step 4: Run test to verify pass**

Run: `go test ./internal/domain/ -run "TestStatusAwaitingRemediation|TestAwaitingRemediation" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/domain/lifecycle.go internal/domain/lifecycle_remediation_test.go
git commit -m "feat: add StatusAwaitingRemediation run status and legal transitions"
```

---

### Task 2: Add SkillRemediation domain type

**Objective:** Define the input type the skill submits.

**Files:**
- Modify: `internal/domain/verification_report.go`

**Step 1: Implement**

In `internal/domain/verification_report.go`, add after `RemediationSpec` (after line 122):

```go
type SkillRemediation struct {
	Needs []RemediationNeed `json:"needs"`
	Graph WorkGraph         `json:"graph"`
}
```

**Step 2: Verify compilation**

Run: `go build ./internal/domain/`
Expected: success

**Step 3: Commit**

```bash
git add internal/domain/verification_report.go
git commit -m "feat: add SkillRemediation domain type for skill-authored remediation"
```

---

### Task 3: Change persistVerificationAttempt fail path to transition to AwaitingRemediation

**Objective:** Stop creating remediation rounds in Go. Transition to `StatusAwaitingRemediation` instead.

**Files:**
- Modify: `internal/workflow/verification.go` (lines 278-286 in `persistVerificationAttempt`)
- Modify: `internal/workflow/service_test.go` (`TestVerificationFailureCreatesOneRemediationRound`)

**Step 1: Update test expectations**

In `internal/workflow/service_test.go`, update `TestVerificationFailureCreatesOneRemediationRound` (line 263):

Change the assertions after `RunVerification` returns (lines 272-289):
- `run.Phase` should be `PhaseConstruction` (unchanged)
- `run.Status` should be `StatusAwaitingRemediation` (was `StatusPending`)
- `len(run.ConstructionRounds)` should be `1` (was `2` — no remediation round created yet)
- Remove the `remediation` variable and the graph leak check (no remediation graph exists yet)
- The rest of the test (Prepare, BindTask, etc.) should be wrapped in a helper that first calls `PutRemediation` to create the remediation round, then proceeds

**Step 2: Run test to verify failure**

Run: `go test ./internal/workflow/ -run TestVerificationFailureCreatesOneRemediationRound -v`
Expected: FAIL — `status should be awaiting_remediation, got pending`

**Step 3: Implement**

In `internal/workflow/verification.go`, `persistVerificationAttempt` (line 278-286), change:
```go
case domain.VerificationFail:
    if attempt.Number == 1 {
        round, err := remediationRound(*run, attempt)
        if err != nil {
            return err
        }
        run.ConstructionRounds = append(run.ConstructionRounds, round)
        run.Phase = domain.PhaseConstruction
        run.Status = domain.StatusPending
    } else {
```
to:
```go
case domain.VerificationFail:
    if attempt.Number == 1 {
        run.Phase = domain.PhaseConstruction
        run.Status = domain.StatusAwaitingRemediation
    } else {
```

**Step 4: Run test to verify pass**

Run: `go test ./internal/workflow/ -run TestVerificationFailureCreatesOneRemediationRound -v`
Expected: PASS (after the test is fully updated to call `PutRemediation` — see Task 5 for the method, so this test may need to be split: first verify the state transition, then the full flow after Task 5 is done)

**Note:** The test update is non-trivial because the existing test proceeds through Prepare → BindTask → etc. after the remediation round. Since `PutRemediation` doesn't exist yet, split the test:
- Part A: Verify fail transitions to `AwaitingRemediation` (no remediation round)
- Part B: After Task 5, add the full flow test (remediation_integration_test.go)

**Step 5: Commit**

```bash
git add internal/workflow/verification.go internal/workflow/service_test.go
git commit -m "feat: persistVerificationAttempt transitions to AwaitingRemediation on fail"
```

---

### Task 4: Change JudgeVerification fail path to transition to AwaitingRemediation

**Objective:** Same change for the judge path.

**Files:**
- Modify: `internal/workflow/verification.go` (lines 815-823 in `JudgeVerification`)
- Modify: `internal/workflow/judge_verification_test.go` (`TestJudgeVerificationFailTriggersRemediation`)
- Modify: `internal/workflow/rubric_integration_test.go` (`TestVerificationRubricFailTriggersRemediation`)

**Step 1: Update test expectations**

In `judge_verification_test.go`, update the fail test:
- `run.Status` should be `StatusAwaitingRemediation` (was `StatusPending`)
- `len(run.ConstructionRounds)` should be `1` (was `2`)

In `rubric_integration_test.go`, update `TestVerificationRubricFailTriggersRemediation`:
- `run.Status` should be `StatusAwaitingRemediation`
- `len(run.ConstructionRounds)` should be `1`

**Step 2: Run test to verify failure**

Run: `go test ./internal/workflow/ -run "TestJudgeVerificationFail|TestVerificationRubricFail" -v`
Expected: FAIL

**Step 3: Implement**

In `internal/workflow/verification.go`, `JudgeVerification` (line 815-823), change:
```go
case domain.VerificationFail:
    if attempt.Number == 1 {
        round, err := remediationRound(*r, *attempt)
        if err != nil {
            return err
        }
        r.ConstructionRounds = append(r.ConstructionRounds, round)
        r.Phase = domain.PhaseConstruction
        r.Status = domain.StatusPending
    } else {
```
to:
```go
case domain.VerificationFail:
    if attempt.Number == 1 {
        r.Phase = domain.PhaseConstruction
        r.Status = domain.StatusAwaitingRemediation
    } else {
```

**Step 4: Run test to verify pass**

Run: `go test ./internal/workflow/ -run "TestJudgeVerificationFail|TestVerificationRubricFail" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/workflow/verification.go internal/workflow/judge_verification_test.go internal/workflow/rubric_integration_test.go
git commit -m "feat: JudgeVerification transitions to AwaitingRemediation on fail"
```

---

### Task 5: Add PutRemediation method to Service

**Objective:** The skill submits a `SkillRemediation` (needs + work graph). Go validates, wraps in `ConstructionState`, persists, transitions to `StatusPending`.

**Files:**
- Modify: `internal/workflow/verification.go`
- Create: `internal/workflow/put_remediation_test.go`

**Step 1: Write failing test**

Create `internal/workflow/put_remediation_test.go`:

```go
package workflow

import (
	"context"
	"testing"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func TestPutRemediationSuccess(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "put-remediation", true)
	completeVerificationCandidate(t, service, execution, profile)

	run, report, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != domain.VerificationFail ||
		run.Status != domain.StatusAwaitingRemediation {
		t.Fatalf("expected fail→awaiting_remediation, got verdict=%s status=%s",
			report.Verdict, run.Status)
	}

	// Skill authors remediation spec.
	spec := domain.SkillRemediation{
		Needs: []domain.RemediationNeed{{
			RequirementIDs:         []string{"req-1"},
			AcceptanceCriterionIDs: []string{"crit-1"},
			Expected:               "the feature should behave correctly",
			Actual:                  "it did not",
		}},
		Graph: domain.WorkGraph{
			SchemaVersion: domain.SchemaVersion,
			Context:       execution,
			Producer:      domain.Producer{Skill: "hermoso-verification", Runtime: "agent"},
			Revision:      1,
			Items: []domain.WorkItem{{
				ID:                "remediation-1",
				Title:             "Fix the failing behavior",
				Prompt:            "The test failed because X. Fix by doing Y.",
				AcceptanceCriteria: []string{"the feature should behave correctly"},
				RequirementIDs:     []string{"req-1"},
				CriterionIDs:       []string{"crit-1"},
				Worker: domain.Worker{
					Profile: "default",
					Skills:  []domain.SkillBinding{{Name: "hermoso-construction"}},
				},
			}},
		},
	}

	data := encode(t, spec)
	run, changed, err := service.PutRemediation(context.Background(), execution, data)
	if err != nil {
		t.Fatalf("PutRemediation failed: %v", err)
	}
	if !changed {
		t.Error("expected changed=true")
	}
	if run.Phase != domain.PhaseConstruction || run.Status != domain.StatusPending {
		t.Fatalf("expected construction/pending, got %s/%s", run.Phase, run.Status)
	}
	remediation := run.LatestConstruction()
	if remediation == nil || remediation.Kind != "remediation" || remediation.Remediation == nil {
		t.Fatalf("expected remediation round, got %#v", remediation)
	}
	if remediation.SourceHash != run.VerificationAttempts[0].ReportHash {
		t.Errorf("SourceHash should match failed report hash")
	}
	if remediation.Remediation.FailedReportHash != run.VerificationAttempts[0].ReportHash {
		t.Errorf("FailedReportHash should match failed report hash")
	}
	if len(remediation.Remediation.Needs) != 1 {
		t.Errorf("expected 1 need, got %d", len(remediation.Remediation.Needs))
	}
	if remediation.Graph.Producer.Runtime != "agent" {
		t.Errorf("expected agent runtime, got %s", remediation.Graph.Producer.Runtime)
	}
}

func TestPutRemediationRejectsWrongState(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "put-remediation-wrong", true)
	completeVerificationCandidate(t, service, execution, profile)

	// Run is in construction/awaiting_verification, not awaiting_remediation.
	spec := domain.SkillRemediation{
		Needs: []domain.RemediationNeed{{
			RequirementIDs: []string{"req-1"},
			Expected:       "x",
			Actual:          "y",
		}},
		Graph: domain.WorkGraph{
			SchemaVersion: domain.SchemaVersion,
			Context:       execution,
			Producer:      domain.Producer{Skill: "hermoso-verification", Runtime: "agent"},
			Revision:      1,
			Items: []domain.WorkItem{{
				ID:    "r-1",
				Title: "fix",
				Prompt: "fix it",
				RequirementIDs: []string{"req-1"},
				Worker: domain.Worker{
					Profile: "default",
					Skills:  []domain.SkillBinding{{Name: "hermoso-construction"}},
				},
			}},
		},
	}
	data := encode(t, spec)
	_, _, err := service.PutRemediation(context.Background(), execution, data)
	if err == nil {
		t.Fatal("expected error when run is not awaiting_remediation")
	}
}

func TestPutRemediationRejectsEmptyNeeds(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "put-remediation-empty", true)
	completeVerificationCandidate(t, service, execution, profile)

	run, _, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.StatusAwaitingRemediation {
		t.Fatalf("expected awaiting_remediation, got %s", run.Status)
	}

	spec := domain.SkillRemediation{
		Needs: []domain.RemediationNeed{},
		Graph: domain.WorkGraph{
			SchemaVersion: domain.SchemaVersion,
			Context:       execution,
			Producer:      domain.Producer{Skill: "hermoso-verification", Runtime: "agent"},
			Revision:      1,
			Items: []domain.WorkItem{{
				ID:    "r-1",
				Title: "fix",
				Prompt: "fix it",
				Worker: domain.Worker{
					Profile: "default",
					Skills:  []domain.SkillBinding{{Name: "hermoso-construction"}},
				},
			}},
		},
	}
	data := encode(t, spec)
	_, _, err = service.PutRemediation(context.Background(), execution, data)
	if err == nil {
		t.Fatal("expected error for empty needs")
	}
}
```

**Step 2: Run test to verify failure**

Run: `go test ./internal/workflow/ -run TestPutRemediation -v`
Expected: FAIL — `PutRemediation` method doesn't exist

**Step 3: Implement PutRemediation**

In `internal/workflow/verification.go`, add method (after `JudgeVerification` or before `remediationRound`):

```go
func (s Service) PutRemediation(
	ctx context.Context,
	execution domain.ContextRef,
	data []byte,
) (domain.Run, bool, error) {
	var spec domain.SkillRemediation
	if err := json.Unmarshal(data, &spec); err != nil {
		return domain.Run{}, false, fmt.Errorf("decode skill remediation: %w", err)
	}
	if len(spec.Needs) == 0 {
		return domain.Run{}, false, errors.New("remediation spec must contain at least one need")
	}
	if !spec.Graph.Context.Equal(execution) {
		return domain.Run{}, false, errors.New("remediation graph context does not match command context")
	}
	if err := spec.Graph.Validate(); err != nil {
		return domain.Run{}, false, fmt.Errorf("remediation graph validation: %w", err)
	}
	graphHash, err := Hash(spec.Graph)
	if err != nil {
		return domain.Run{}, false, err
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
		if r.Phase != domain.PhaseConstruction || r.Status != domain.StatusAwaitingRemediation {
			return errors.New("remediation requires a run in awaiting_remediation state")
		}
		if len(r.VerificationAttempts) == 0 {
			return errors.New("no verification attempt to remediate")
		}
		attempt := r.VerificationAttempts[len(r.VerificationAttempts)-1]
		remediationSpec := domain.RemediationSpec{
			SchemaVersion:    domain.SchemaVersion,
			Context:          r.Context,
			FailedReportHash: attempt.ReportHash,
			Needs:            spec.Needs,
			CreatedAt:        attempt.Report.CompletedAt,
		}
		round := domain.ConstructionState{
			Number:      uint64(len(r.ConstructionRounds) + 1),
			Kind:        "remediation",
			SourceHash:  attempt.ReportHash,
			Remediation: &remediationSpec,
			Graph:       spec.Graph,
			Hash:        graphHash,
		}
		r.ConstructionRounds = append(r.ConstructionRounds, round)
		r.Phase = domain.PhaseConstruction
		r.Status = domain.StatusPending
		r.Revision++
		r.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, changed, err
}
```

**Step 4: Run test to verify pass**

Run: `go test ./internal/workflow/ -run TestPutRemediation -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/workflow/verification.go internal/workflow/put_remediation_test.go
git commit -m "feat: add PutRemediation method for skill-authored remediation spec"
```

---

### Task 6: Remove remediationRound function

**Objective:** Dead code removal — `remediationRound` is no longer called.

**Files:**
- Modify: `internal/workflow/verification.go`

**Step 1: Verify remediationRound is unused**

Run: `grep -n "remediationRound" internal/workflow/verification.go`
Expected: only the function definition (no call sites)

**Step 2: Remove the function**

Delete `remediationRound` (lines 584-654) from `internal/workflow/verification.go`.

**Step 3: Verify compilation and tests**

Run: `go test ./internal/workflow/ -v 2>&1 | tail -20`
Expected: PASS (all tests)

**Step 4: Commit**

```bash
git add internal/workflow/verification.go
git commit -m "refactor: remove dead remediationRound function"
```

---

### Task 7: Update TestVerificationFailureCreatesOneRemediationRound to full two-phase flow

**Objective:** The existing test was split in Task 3. Now complete it by calling `PutRemediation` and proceeding through the rest of the flow.

**Files:**
- Modify: `internal/workflow/service_test.go` (`TestVerificationFailureCreatesOneRemediationRound`)

**Step 1: Update test**

The test should:
1. Run verification → fail → `{PhaseConstruction, StatusAwaitingRemediation}`
2. Verify no remediation round exists yet
3. Call `PutRemediation` with a skill-authored spec → `{PhaseConstruction, StatusPending}`
4. Verify remediation round exists with correct fields
5. Verify the remediation graph doesn't leak verification assets
6. Proceed with Prepare → BindTask → StartWork → FinishWork → Integrate → PutResult → RunVerification (second attempt)

**Step 2: Run test**

Run: `go test ./internal/workflow/ -run TestVerificationFailureCreatesOneRemediationRound -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/workflow/service_test.go
git commit -m "test: update remediation test for two-phase flow with skill-authored spec"
```

---

### Task 8: Add integration test for full two-phase remediation flow

**Objective:** End-to-end test: verification fail → awaiting_remediation → put remediation → construction → verification (second attempt).

**Files:**
- Create: `internal/workflow/remediation_integration_test.go`

**Step 1: Write test**

Test the full flow:
1. Setup: design with verification contract, construction candidate completed
2. `RunVerification` → fail → `{PhaseConstruction, StatusAwaitingRemediation}`
3. `PutRemediation` with skill-authored spec → `{PhaseConstruction, StatusPending}`
4. `Prepare` → workspace created
5. `BindTask` → `StartWork` → `FinishWork` → `Integrate` → `PutResult`
6. `RunVerification` (second attempt) → pass or fail

**Step 2: Run test**

Run: `go test ./internal/workflow/ -run TestRemediationIntegration -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/workflow/remediation_integration_test.go
git commit -m "test: add integration test for full two-phase remediation flow"
```

---

### Task 9: Add verification remediate CLI subcommand

**Objective:** `hermoso verification remediate <context> <spec-path>` reads a JSON file and calls `PutRemediation`.

**Files:**
- Modify: `internal/app/app.go` (`runVerificationContract`)
- Create: `internal/app/remediate_cmd_test.go`

**Step 1: Write failing test**

Create `internal/app/remediate_cmd_test.go`:

```go
package app

import (
	"context"
	"path/filepath"
	"testing"
)

func TestVerificationRemediateUsageError(t *testing.T) {
	deps := testDeps(t)
	if code := Run(context.Background(), []string{"verification", "bogus", "p", "f", "r", deps.repo.Root}, deps); code != ExitUsage {
		t.Fatalf("expected ExitUsage, got %d", code)
	}
}
```

**Step 2: Run test to verify failure**

Run: `go test ./internal/app/ -run TestVerificationRemediate -v`
Expected: depends — if "bogus" is already rejected, test passes. If not, adjust.

**Step 3: Implement**

In `internal/app/app.go`, modify `runVerificationContract`:

1. Change the subcommand check (line 456) from:
```go
if len(args) == 0 || (args[0] != "put" && args[0] != "run" && args[0] != "judge") {
```
to:
```go
if len(args) == 0 || (args[0] != "put" && args[0] != "run" && args[0] != "judge" && args[0] != "remediate") {
```

2. Update the usage message to include `remediate`.

3. Add the `remediate` action handler (after the `judge` handler, before the `put` handler):
```go
if action == "remediate" {
    if len(rest) != 1 {
        return a.out.usageError("verification remediate requires exactly one spec JSON path after full context")
    }
    data, err := a.deps.FS.ReadFile(rest[0])
    if err != nil {
        return a.out.failure(ExitFailure, ErrorInternal, err.Error())
    }
    run, changed, err := service.PutRemediation(ctx, execution, data)
    if err != nil {
        return a.out.failure(ExitFailure, ErrorState, err.Error())
    }
    return a.out.success(
        "verification remediate",
        map[string]any{"run": run, "changed": changed},
        fmt.Sprintf("remediation round %d persisted\n", len(run.ConstructionRounds)),
    )
}
```

4. Update the help text (line 48):
```go
"  hermoso verification <put|run|judge|remediate> ..."
```

**Step 4: Run test to verify pass**

Run: `go test ./internal/app/ -run TestVerificationRemediate -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/app/app.go internal/app/remediate_cmd_test.go
git commit -m "feat: add verification remediate CLI command for skill-authored remediation"
```

---

### Task 10: Update hermoso-verification SKILL.md with two-phase remediation flow

**Objective:** Document the new two-phase remediation flow.

**Files:**
- Modify: `skills/hermoso-verification/SKILL.md`

**Step 1: Update SKILL.md**

Add a section documenting:
- Phase 1: When `verification run` or `verification judge` returns a fail, the run is in `awaiting_remediation`
- The skill reads the failed report + evidence from the command output
- The skill authors a `SkillRemediation` JSON file with:
  - `needs`: one or more `RemediationNeed` entries explaining what went wrong (Expected, Actual, surface/requirement/criterion IDs)
  - `graph`: a `WorkGraph` with targeted work items (specific prompts, correct workers, dependencies)
  - The `Producer` should be `{Skill: "hermoso-verification", Runtime: "agent"}`
- Phase 2: Call `hermoso verification remediate <context> <spec-path>` to submit
- Go validates and persists, transitioning to `construction/pending`

**Step 2: Commit**

```bash
git add skills/hermoso-verification/SKILL.md
git commit -m "docs: update hermoso-verification SKILL.md with two-phase remediation flow"
```

---

### Task 11: Mark Issue B as resolved in critique document

**Objective:** Update the critique doc.

**Files:**
- Modify: `docs/verification-loop-critique.md`

**Step 1: Update critique doc**

1. Update header: add "Updated: 2026-08-10 — Issue B resolved"
2. Change Issue B heading to "RESOLVED"
3. Add resolution description:
   - Two-phase remediation: Go transitions to `StatusAwaitingRemediation` on fail, skill authors remediation spec via `PutRemediation`
   - `remediationRound` function removed — Go no longer templates remediation needs or work items
   - `Producer` on remediation graph is now `{Skill: "hermoso-verification", Runtime: "agent"}` — honestly reflecting skill authorship
4. Update summary table: `remediationRound` row shows "Transitions to AwaitingRemediation, accepts skill-authored spec"

**Step 2: Commit**

```bash
git add docs/verification-loop-critique.md
git commit -m "docs: mark Issue B as resolved in verification-loop-critique.md"
```

---

### Task 12: Full test suite and final commit

**Objective:** Verify no regressions.

**Step 1: Run full test suite**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./... -v 2>&1 | tail -30`
Expected: all tests pass

**Step 2: Verify no dead code**

Run: `grep -rn "remediationRound" internal/`
Expected: no matches

**Step 3: Final status check**

Run: `git log --oneline -15`
Expected: 10-12 commits for Issue B

---

## Risks and Tradeoffs

### Non-disclosure of verification assets
The skill has access to the full failed report (including evidence — stdout, stderr, commands). The skill is trusted to not leak verification asset references into the remediation work items. This is operational non-disclosure, same as the current design. The existing test that checks the remediation graph for leaked asset strings should be maintained.

### Breaking change to existing test flows
`TestVerificationFailureCreatesOneRemediationRound` is a large test that proceeds through the entire remediation → second verification flow. Splitting it requires care to maintain test coverage.

### No Go-side content sanitization
Go does not sanitize the skill-authored graph content. If the skill includes hidden asset references, they would be persisted. This is acceptable because: (1) the skill is the `hermoso-verification` skill which is trusted, (2) the graph is validated structurally, (3) operational non-disclosure is the existing pattern.

### Worker assignment
Previously, `remediationRound` copied the worker from the previous construction round. Now the skill must explicitly set the worker on each work item. The skill should be instructed to do this in SKILL.md.
