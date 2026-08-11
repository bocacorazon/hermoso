# Issue A Remediation: Split Rubric Judgment to Skill

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Move rubric oracle evaluation out of Go's `judgeExecution` (which reduces qualitative rubrics to `exitCode == 0`) and into the `hermoso-verification` skill, establishing a two-phase verification flow: `verification run` (evidence collection + mechanical oracle evaluation) → `verification judge` (skill applies rubric criteria to collected evidence).

**Architecture:** Go remains the sole authority for evidence collection (command execution, stdout/stderr hashing, worktree fingerprinting) and mechanical oracle evaluation (exit_code, stdout_regex, file, json_path). For rubric oracle types, Go marks the outcome as `pending` and persists the attempt in an `awaiting_judgment` state. The `hermoso-verification` skill then reads the pending outcomes, applies the rubric criteria to the collected evidence, and submits judgments via a new `hermoso verification judge` command. Go validates the skill's judgments (correct judgment IDs, valid statuses, evidence hashes match the persisted outcome) before completing the attempt and proceeding with publication/remediation/blocking.

**Tech Stack:** Go 1.26+, existing Hermoso domain/workflow/app packages, JSON-based skill definitions.

---

## Current State

The `judgeExecution` function in `internal/workflow/verification.go:358-416` handles all oracle types in a single switch. The `rubric` and `gherkin` cases share a branch:

```go
case "gherkin", "rubric":
    if exitCode == 0 {
        return domain.JudgmentPass, "approved external judge passed"
    }
```

This reduces rubric judgments — which the contract specifies as having a `RubricPolicy` with `Judge` and `Criteria` fields requiring qualitative evaluation — to a simple exit-code check. The rubric criteria and judge identity recorded in the contract's `RubricPolicy` are never used for evaluation; they are only recorded as metadata in the evidence struct.

The `gherkin` oracle type is different: a BDD runner's exit code IS a legitimate mechanical oracle. If the Cucumber scenarios pass, exit 0; if they fail, non-zero. The fix leaves `gherkin` as mechanical and only moves `rubric` to the skill.

## Design Decision: Gherkin Stays Mechanical

**Rationale:** A Gherkin runner (Cucumber, godog, etc.) IS the oracle — its exit code deterministically reflects whether the scenarios passed. There is no qualitative judgment to apply. The critique's "arguably open-ended BDD" caveat is a possible future enhancement, not part of Issue A. Moving only `rubric` to the skill keeps the change focused and backward-compatible: existing BDD verification runs that produce no rubric judgments behave exactly as before (single-phase, no `awaiting_judgment` state).

## Files Likely to Change

- `internal/domain/verification_report.go` — new status, verdict, types
- `internal/domain/lifecycle.go` — new run status and transitions
- `internal/workflow/verification.go` — split judgeExecution, update RunVerification, add JudgeVerification
- `internal/app/app.go` — new `verification judge` CLI command
- `skills/hermoso-verification/SKILL.md` — updated verification lifecycle for two-phase flow
- `internal/workflow/service_test.go` — new tests for two-phase rubric verification

---

### Task 1: Add JudgmentPending Status and VerificationPending Verdict

**Objective:** Add new domain constants for the pending judgment status and pending verification verdict.

**Files:**
- Modify: `internal/domain/verification_report.go:8-23`

**Step 1: Write failing test**

Create `internal/domain/verification_report_pending_test.go`:

```go
package domain

import "testing"

func TestJudgmentPendingConstant(t *testing.T) {
	if JudgmentPending != JudgmentStatus("pending") {
		t.Fatalf("JudgmentPending = %q, want %q", JudgmentPending, "pending")
	}
}

func TestVerificationPendingConstant(t *testing.T) {
	if VerificationPending != VerificationVerdict("pending") {
		t.Fatalf("VerificationPending = %q, want %q", VerificationPending, "pending")
	}
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestJudgmentPending -v`
Expected: FAIL — `undefined: JudgmentPending`

**Step 3: Write minimal implementation**

In `internal/domain/verification_report.go`, add `VerificationPending` to the verdict constants:

```go
const (
	VerificationPass         VerificationVerdict = "pass"
	VerificationFail         VerificationVerdict = "fail"
	VerificationBlocked      VerificationVerdict = "blocked"
	VerificationInconclusive VerificationVerdict = "inconclusive"
	VerificationPending      VerificationVerdict = "pending"
)
```

Add `JudgmentPending` to the status constants:

```go
const (
	JudgmentPass    JudgmentStatus = "pass"
	JudgmentFail    JudgmentStatus = "fail"
	JudgmentBlocked JudgmentStatus = "blocked"
	JudgmentPending JudgmentStatus = "pending"
)
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestJudgmentPending -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/domain/verification_report.go internal/domain/verification_report_pending_test.go
git commit -m "feat: add JudgmentPending status and VerificationPending verdict"
```

---

### Task 2: Add QualitativeJudgment and SkillJudgment Types

**Objective:** Add the `QualitativeJudgment` struct (recorded on an outcome when the skill applies a rubric) and the `SkillJudgment` input type (submitted by the skill via the judge command).

**Files:**
- Modify: `internal/domain/verification_report.go` (add types after `JudgmentOutcome`)
- Test: `internal/domain/verification_report_pending_test.go`

**Step 1: Write failing test**

Append to `internal/domain/verification_report_pending_test.go`:

```go
func TestQualitativeJudgmentJSONRoundTrip(t *testing.T) {
	original := QualitativeJudgment{
		Judge:     "hermoso-verification",
		Reasoning: "output matches all rubric criteria",
		Criteria:  []string{"correctness", "completeness"},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored QualitativeJudgment
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Judge != original.Judge || restored.Reasoning != original.Reasoning ||
		len(restored.Criteria) != 2 {
		t.Fatalf("round-trip mismatch: %+v", restored)
	}
}

func TestSkillJudgmentJSONRoundTrip(t *testing.T) {
	original := SkillJudgment{
		JudgmentID: "judgment-rubric-1",
		Status:     JudgmentPass,
		Summary:    "all criteria satisfied",
		Qualitative: &QualitativeJudgment{
			Judge:    "hermoso-verification",
			Reasoning: "output matches criteria",
			Criteria:  []string{"correctness"},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored SkillJudgment
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.JudgmentID != original.JudgmentID || restored.Status != original.Status {
		t.Fatalf("round-trip mismatch: %+v", restored)
	}
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestQualitativeJudgment -v`
Expected: FAIL — `undefined: QualitativeJudgment`

**Step 3: Write minimal implementation**

In `internal/domain/verification_report.go`, add after the `JudgmentOutcome` struct (line 47):

```go
type QualitativeJudgment struct {
	Judge     string   `json:"judge"`
	Reasoning string   `json:"reasoning"`
	Criteria  []string `json:"criteria"`
}

type SkillJudgment struct {
	JudgmentID   string                `json:"judgment_id"`
	Status       JudgmentStatus         `json:"status"`
	Summary      string                `json:"summary"`
	Qualitative  *QualitativeJudgment   `json:"qualitative,omitempty"`
}
```

Also add `QualitativeJudgment` field to `JudgmentOutcome`:

```go
type JudgmentOutcome struct {
	JudgmentID             string               `json:"judgment_id"`
	Status                 JudgmentStatus       `json:"status"`
	RequirementIDs         []string             `json:"requirement_ids"`
	AcceptanceCriterionIDs []string             `json:"acceptance_criterion_ids"`
	SurfaceIDs             []string             `json:"surface_ids"`
	Summary                string               `json:"summary"`
	Evidence               VerificationEvidence `json:"evidence"`
	QualitativeJudgment    *QualitativeJudgment `json:"qualitative_judgment,omitempty"`
}
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestQualitativeJudgment -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/domain/verification_report.go internal/domain/verification_report_pending_test.go
git commit -m "feat: add QualitativeJudgment and SkillJudgment domain types"
```

---

### Task 3: Update VerificationReport.Validate() to Accept Pending Status and Verdict

**Objective:** The report validator currently rejects any status that is not pass/fail/blocked. It must accept `pending` for both individual outcomes and the report verdict. Pending outcomes must still have valid evidence (hashes, command) since Go collected the evidence before deferring judgment.

**Files:**
- Modify: `internal/domain/verification_report.go:117-184` (Validate method)
- Test: `internal/domain/verification_report_pending_test.go`

**Step 1: Write failing test**

Append to `internal/domain/verification_report_pending_test.go`:

```go
func TestReportValidateAcceptsPendingOutcome(t *testing.T) {
	report := validPendingReport()
	if err := report.Validate(); err != nil {
		t.Fatalf("pending report should validate: %v", err)
	}
}

func TestReportValidateRejectsPendingOutcomeWithoutSummary(t *testing.T) {
	report := validPendingReport()
	report.Outcomes[0].Summary = ""
	if err := report.Validate(); err == nil {
		t.Fatal("pending outcome without summary should fail validation")
	}
}

func TestReportValidatePendingVerdictRequiresPendingOutcome(t *testing.T) {
	report := validPendingReport()
	report.Outcomes[0].Status = JudgmentPass
	report.Verdict = VerificationPending
	if err := report.Validate(); err == nil {
		t.Fatal("pending verdict with no pending outcomes should fail")
	}
}

func validPendingReport() VerificationReport {
	return VerificationReport{
		SchemaVersion:     SchemaVersion,
		Context:           testContextRef(),
		Producer:          Producer{Skill: "hermoso-verification", Runtime: "deterministic"},
		Attempt:           1,
		CandidateCommit:   "0123456789abcdef0123456789abcdef01234567",
		DesignPackageHash: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CandidateModel: ModelReference{
			SnapshotID:     "snap-1",
			SourceRevision: "0123456789abcdef0123456789abcdef01234567",
		},
		Outcomes: []JudgmentOutcome{{
			JudgmentID:             "judgment-1",
			Status:                 JudgmentPending,
			RequirementIDs:         []string{"req-1"},
			AcceptanceCriterionIDs: []string{"ac-1"},
			SurfaceIDs:             []string{"surface-1"},
			Summary:                "pending qualitative judgment",
			Evidence: VerificationEvidence{
				Command:    []string{"echo", "test"},
				ExitCode:   0,
				StdoutHash: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				StderrHash: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		}},
		Requirements:       []CoverageOutcome{{ID: "req-1", Status: "pending"}},
		AcceptanceCriteria: []CoverageOutcome{{ID: "ac-1", Status: "pending"}},
		Verdict:            VerificationPending,
		StartedAt:          time.Now(),
		CompletedAt:        time.Now(),
	}
}
```

You will also need to add `"time"` and `"encoding/json"` imports and a `testContextRef` helper at the top of the test file:

```go
import (
	"encoding/json"
	"testing"
	"time"
)

func testContextRef() ContextRef {
	return ContextRef{
		ProjectID:  "proj-1",
		FeatureID:  "feat-1",
		RunID:      "run-1",
		Repository: "github.com/test/repo",
	}
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestReportValidate -v`
Expected: FAIL — `must be pass, fail, or blocked`

**Step 3: Write minimal implementation**

In `internal/domain/verification_report.go`, update the outcome status validation at line 147:

```go
if outcome.Status != JudgmentPass && outcome.Status != JudgmentFail &&
	outcome.Status != JudgmentBlocked && outcome.Status != JudgmentPending {
	errs.add(path+".status", "must be pass, fail, blocked, or pending")
}
```

Add pending tracking in the loop (after line 151):

```go
hasPending = hasPending || outcome.Status == JudgmentPending
```

Add `hasPending` declaration at line 139 (after `hasFailure, hasBlocked`):

```go
hasFailure, hasBlocked, hasPending := false, false, false
```

Update the verdict validation at line 166-168 to accept pending:

```go
if r.Verdict != VerificationPass && r.Verdict != VerificationFail &&
	r.Verdict != VerificationBlocked && r.Verdict != VerificationInconclusive &&
	r.Verdict != VerificationPending {
	errs.add("verdict", "must be pass, fail, blocked, inconclusive, or pending")
}
```

Add pending verdict consistency checks (after the existing blocked check at line 178):

```go
if r.Verdict == VerificationPending && !hasPending {
	errs.add("verdict", "pending requires at least one pending judgment")
}
if r.Verdict == VerificationPass && hasPending {
	errs.add("verdict", "pass requires every judgment to pass (no pending)")
}
if r.Verdict == VerificationFail && (hasPending || hasBlocked) {
	errs.add("verdict", "fail requires at least one failure and no pending or blocked judgment")
}
```

Update the candidate model check at line 132 to allow pending:

```go
if r.Verdict != VerificationBlocked && r.Verdict != VerificationPending &&
	r.CandidateModel.SourceRevision != r.CandidateCommit {
	errs.add("candidate_model.source_revision", "must match candidate_commit for completed verification")
}
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestReportValidate -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/domain/verification_report.go internal/domain/verification_report_pending_test.go
git commit -m "feat: accept pending status and verdict in VerificationReport validation"
```

---

### Task 4: Update validateCoverageOutcomes to Accept Pending Status

**Objective:** Coverage outcomes derived from pending judgments should have status "pending", which the validator must accept.

**Files:**
- Modify: `internal/domain/verification_report.go:187-207` (validateCoverageOutcomes)
- Test: `internal/domain/verification_report_pending_test.go`

**Step 1: Write failing test**

Append to `internal/domain/verification_report_pending_test.go`:

```go
func TestValidateCoverageOutcomesAcceptsPending(t *testing.T) {
	errs := &ValidationErrors{}
	validateCoverageOutcomes("requirements", []CoverageOutcome{
		{ID: "req-1", Status: "pending"},
	}, errs)
	if len(errs.Errors) != 0 {
		t.Fatalf("expected no errors for pending coverage, got: %v", errs.Errors)
	}
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestValidateCoverageOutcomesAcceptsPending -v`
Expected: FAIL — `must be pass, fail, blocked, or excluded`

**Step 3: Write minimal implementation**

In `internal/domain/verification_report.go`, update line 199-201 in `validateCoverageOutcomes`:

```go
if outcome.Status != string(JudgmentPass) && outcome.Status != string(JudgmentFail) &&
	outcome.Status != string(JudgmentBlocked) && outcome.Status != string(JudgmentPending) &&
	outcome.Status != "excluded" {
	errs.add(itemPath+".status", "must be pass, fail, blocked, pending, or excluded")
}
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestValidateCoverageOutcomesAcceptsPending -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/domain/verification_report.go internal/domain/verification_report_pending_test.go
git commit -m "feat: accept pending status in coverage outcome validation"
```

---

### Task 5: Add StatusAwaitingJudgment Run Status and Legal Transitions

**Objective:** The run lifecycle needs a new status `awaiting_judgment` in the verification phase, entered after `verification run` produces pending rubric outcomes and exited by `verification judge`.

**Files:**
- Modify: `internal/domain/lifecycle.go` (add constant, update `statusAllowed`, add transitions)
- Test: `internal/domain/lifecycle_test.go` (or `internal/domain/verification_report_pending_test.go`)

**Step 1: Write failing test**

Append to `internal/domain/verification_report_pending_test.go`:

```go
func TestStatusAwaitingJudgmentConstant(t *testing.T) {
	if StatusAwaitingJudgment != RunStatus("awaiting_judgment") {
		t.Fatalf("StatusAwaitingJudgment = %q, want %q", StatusAwaitingJudgment, "awaiting_judgment")
	}
}

func TestAwaitingJudgmentTransitionsToInProgress(t *testing.T) {
	from := lifecycleState{PhaseVerification, StatusAwaitingJudgment}
	to := lifecycleState{PhaseVerification, StatusInProgress}
	if !legalTransition(from, to) {
		t.Fatal("awaiting_judgment should transition to in_progress")
	}
}

func TestAwaitingJudgmentTransitionsToBlocked(t *testing.T) {
	from := lifecycleState{PhaseVerification, StatusAwaitingJudgment}
	to := lifecycleState{PhaseVerification, StatusBlocked}
	if !legalTransition(from, to) {
		t.Fatal("awaiting_judgment should transition to blocked")
	}
}

func TestAwaitingJudgmentTransitionsToAwaitingRelease(t *testing.T) {
	from := lifecycleState{PhaseVerification, StatusAwaitingJudgment}
	to := lifecycleState{PhaseVerification, StatusAwaitingRelease}
	if !legalTransition(from, to) {
		t.Fatal("awaiting_judgment should transition to awaiting_release")
	}
}

func TestAwaitingJudgmentTransitionsToPending(t *testing.T) {
	from := lifecycleState{PhaseVerification, StatusAwaitingJudgment}
	to := lifecycleState{PhaseConstruction, StatusPending}
	if !legalTransition(from, to) {
		t.Fatal("awaiting_judgment should transition to construction pending (remediation)")
	}
}

func TestAwaitingJudgmentAllowedInVerificationPhase(t *testing.T) {
	if !statusAllowed(PhaseVerification, StatusAwaitingJudgment) {
		t.Fatal("awaiting_judgment should be allowed in verification phase")
	}
}
```

Note: `legalTransition` is a package-private function. The test must be in `package domain`. Check if the function is called `legalTransition` or if there's a public wrapper. If it's the map `legalTransitions`, the test can check directly:

```go
func TestAwaitingJudgmentTransitionsToInProgress(t *testing.T) {
	from := lifecycleState{PhaseVerification, StatusAwaitingJudgment}
	to := lifecycleState{PhaseVerification, StatusInProgress}
	if _, ok := legalTransitions[from][to]; !ok {
		t.Fatal("awaiting_judgment should transition to in_progress")
	}
}
```

Use the map-check form. Also check that `statusAllowed` is the correct function name by reading the lifecycle file.

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestAwaitingJudgment -v`
Expected: FAIL — `undefined: StatusAwaitingJudgment`

**Step 3: Write minimal implementation**

In `internal/domain/lifecycle.go`, add the constant alongside other run statuses:

```go
StatusAwaitingJudgment RunStatus = "awaiting_judgment"
```

In `statusAllowed`, add to the `PhaseVerification` case:

```go
case PhaseVerification:
	return status == StatusPending || status == StatusInProgress ||
		status == StatusCompleted || status == StatusAwaitingRelease ||
		status == StatusAwaitingJudgment
```

In `legalTransitions`, add a new entry:

```go
{PhaseVerification, StatusAwaitingJudgment}: {
	{PhaseVerification, StatusInProgress}: {},
	{PhaseVerification, StatusAwaitingRelease}: {},
	{PhaseVerification, StatusBlocked}: {},
	{PhaseConstruction, StatusPending}: {},
	{PhaseVerification, StatusCancelled}: {},
},
```

Also update the `PhaseVerification, StatusInProgress` entry to allow transitioning to `awaiting_judgment`:

```go
{PhaseVerification, StatusInProgress}: {
	{PhaseVerification, StatusBlocked}: {},
	{PhaseVerification, StatusAwaitingRelease}: {},
	{PhaseVerification, StatusAwaitingJudgment}: {},
	{PhaseVerification, StatusCancelled}: {},
},
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/domain/ -run TestAwaitingJudgment -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/domain/lifecycle.go internal/domain/verification_report_pending_test.go
git commit -m "feat: add StatusAwaitingJudgment run status and legal transitions"
```

---

### Task 6: Update judgeExecution to Return Pending for Rubric Oracle

**Objective:** Remove `rubric` from the shared `gherkin/rubric` case in `judgeExecution`. Rubric judgments return `JudgmentPending` — Go has collected the evidence (stdout, stderr, exit code, hashes) but defers the pass/fail judgment to the skill. The `gherkin` case stays as-is (exit code is a legitimate mechanical oracle for BDD runners).

**Files:**
- Modify: `internal/workflow/verification.go:358-416` (judgeExecution function)
- Test: `internal/workflow/service_test.go`

**Step 1: Write failing test**

Add a new test to `internal/workflow/service_test.go`:

```go
func TestVerificationRubricJudgmentReturnsPending(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "verification-rubric-pending", false)
	// Override the verification package to use a rubric judgment
	run := completeRubricVerificationCandidate(t, service, execution, profile)

	run, report, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != domain.VerificationPending {
		t.Fatalf("expected pending verdict, got %s", report.Verdict)
	}
	if run.Status != domain.StatusAwaitingJudgment {
		t.Fatalf("expected awaiting_judgment status, got %s", run.Status)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != domain.JudgmentPending {
		t.Fatalf("expected one pending outcome, got %+v", report.Outcomes)
	}
	if report.Outcomes[0].Summary != "pending qualitative judgment" {
		t.Fatalf("expected pending summary, got %q", report.Outcomes[0].Summary)
	}
	// Evidence should still be collected
	if report.Outcomes[0].Evidence.StdoutHash == "" {
		t.Fatal("pending outcome should have evidence hashes")
	}
}
```

You will need a `completeRubricVerificationCandidate` helper. This is a variant of `completeVerificationCandidate` that creates a verification package with a rubric judgment instead of a BDD judgment. Add this helper:

```go
func completeRubricVerificationCandidate(
	t *testing.T,
	service Service,
	execution domain.ContextRef,
	profile string,
) domain.Run {
	t.Helper()
	design := testDesign(t, execution, 1, "Verify with rubric")
	design.Surfaces[0].Title = "FeatureHandler"
	run, _, err := service.PutDesign(context.Background(), execution, encode(t, design))
	if err != nil {
		t.Fatal(err)
	}
	// Use a simple command that exits 0 — Go will collect evidence,
	// but the rubric judgment should be pending regardless of exit code
	run = putRubricVerificationPackage(t, service, execution, run, []string{"go", "test", "./..."})
	if _, _, err := service.ApproveDesign(
		context.Background(), execution, run.Design.Revision, run.Design.PackageHash, "developer", "",
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.PutGraph(
		context.Background(), execution,
		encode(t, testGraph(execution, []domain.WorkItem{testItem("root", nil)})),
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Prepare(context.Background(), execution, profile); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.BindTask(context.Background(), execution, "root", "task-root"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.StartWork(context.Background(), execution, "root"); err != nil {
		t.Fatal(err)
	}
	item := itemState(t, service, execution, "root")
	gitAt(t, item.Workspace.Path, "commit", "--allow-empty", "-m", "complete candidate")
	if _, _, err := service.FinishWork(
		context.Background(), execution, "root", domain.WorkCompleted,
		testEvidence(execution, "candidate-done"), "",
	); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Integrate(context.Background(), execution, nil); err != nil {
		t.Fatal(err)
	}
	run, _, err = service.PutResult(
		context.Background(), execution,
		encode(t, testResult(execution, domain.ResultCompleted, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func putRubricVerificationPackage(
	t *testing.T,
	service Service,
	execution domain.ContextRef,
	run domain.Run,
	command []string,
) domain.Run {
	t.Helper()
	// Rubric judgments don't need Gherkin artifacts, but the contract
	// requires at least one artifact. Use a probe artifact.
	probe := []byte(`probe output`)
	assets := map[string][]byte{"hidden/probe.txt": probe}
	contract := domain.FeatureVerificationContract{
		SchemaVersion: domain.SchemaVersion,
		Context:       execution,
		Producer: domain.Producer{
			Skill: "hermoso-verification-author", Runtime: "test",
		},
		Revision: 1,
		FeatureDesign: domain.ContractReference{
			Context: execution, Kind: "feature-design", Path: "design.json",
			Revision: run.Design.Feature.Revision, Hash: run.Design.FeatureHash,
		},
		BaseModel: run.Design.Feature.BaseModel,
		Artifacts: []domain.VerificationArtifact{{
			ID:          "artifact-probe",
			Kind:        "probe",
			Path:        "hidden/probe.txt",
			ContentHash: digest.Bytes(probe),
		}},
		Judgments: []domain.VerificationJudgment{{
			ID:                     "judgment-rubric",
			Title:                  "Feature meets rubric criteria",
			Modality:               "rubric",
			RequirementIDs:         []string{"req-feature"},
			AcceptanceCriterionIDs: []string{"ac-feature"},
			SurfaceIDs:             []string{"surface-feature"},
			ArtifactIDs:            []string{"artifact-probe"},
			Execution: domain.VerificationExecution{
				Command: command, TimeoutSeconds: 300,
			},
			Oracle:           domain.VerificationOracle{Type: "rubric"},
			RequiredEvidence: []string{"command output"},
			Rubric: &domain.RubricPolicy{
				Judge:    "hermoso-verification",
				Criteria: []string{"correctness", "completeness"},
			},
		}},
		Aggregation: domain.VerificationAggregation{Strategy: "all_required"},
	}
	persisted, changed, err := service.PutVerificationContract(
		context.Background(), execution, encode(t, contract), assets,
	)
	if err != nil || !changed {
		t.Fatalf("put rubric verification package: changed=%v err=%v", changed, err)
	}
	return persisted
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestVerificationRubricJudgmentReturnsPending -v`
Expected: FAIL — the rubric judgment will currently return `JudgmentPass` with "approved external judge passed" because `exitCode == 0`.

**Step 3: Write minimal implementation**

In `internal/workflow/verification.go`, update `judgeExecution` at line 372-376. Change:

```go
switch judgment.Oracle.Type {
case "gherkin", "rubric":
	if exitCode == 0 {
		return domain.JudgmentPass, "approved external judge passed"
	}
```

To:

```go
switch judgment.Oracle.Type {
case "rubric":
	return domain.JudgmentPending, "pending qualitative judgment"
case "gherkin":
	if exitCode == 0 {
		return domain.JudgmentPass, "approved external judge passed"
	}
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestVerificationRubricJudgmentReturnsPending -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/workflow/verification.go internal/workflow/service_test.go
git commit -m "feat: return JudgmentPending for rubric oracle type in judgeExecution"
```

---

### Task 7: Update aggregateVerdict to Handle Pending Outcomes

**Objective:** The verdict aggregator must produce `VerificationPending` when any outcome is pending (and no outcome is blocked, which takes priority).

**Files:**
- Modify: `internal/workflow/verification.go:476-487` (aggregateVerdict)
- Test: `internal/workflow/service_test.go`

**Step 1: Write failing test**

Add to `internal/workflow/service_test.go`:

```go
func TestAggregateVerdictPending(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{Status: domain.JudgmentPass},
		{Status: domain.JudgmentPending},
	}
	if v := aggregateVerdict(outcomes); v != domain.VerificationPending {
		t.Fatalf("expected pending, got %s", v)
	}
}

func TestAggregateVerdictBlockedOverridesPending(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{Status: domain.JudgmentPending},
		{Status: domain.JudgmentBlocked},
	}
	if v := aggregateVerdict(outcomes); v != domain.VerificationBlocked {
		t.Fatalf("expected blocked, got %s", v)
	}
}

func TestAggregateVerdictFailDoesNotOverridePending(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{Status: domain.JudgmentFail},
		{Status: domain.JudgmentPending},
	}
	if v := aggregateVerdict(outcomes); v != domain.VerificationPending {
		t.Fatalf("expected pending (fail cannot override pending), got %s", v)
	}
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestAggregateVerdict -v`
Expected: FAIL — `aggregateVerdict` currently returns `VerificationFail` when there's a failure, regardless of pending.

**Step 3: Write minimal implementation**

Replace `aggregateVerdict` in `internal/workflow/verification.go:476-487`:

```go
func aggregateVerdict(outcomes []domain.JudgmentOutcome) domain.VerificationVerdict {
	hasPending, hasFail := false, false
	for _, outcome := range outcomes {
		switch outcome.Status {
		case domain.JudgmentBlocked:
			return domain.VerificationBlocked
		case domain.JudgmentPending:
			hasPending = true
		case domain.JudgmentFail:
			hasFail = true
		}
	}
	switch {
	case hasPending:
		return domain.VerificationPending
	case hasFail:
		return domain.VerificationFail
	default:
		return domain.VerificationPass
	}
}
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestAggregateVerdict -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/workflow/verification.go internal/workflow/service_test.go
git commit -m "feat: aggregateVerdict returns VerificationPending for pending outcomes"
```

---

### Task 8: Update aggregateCoverage to Handle Pending Outcomes

**Objective:** Coverage outcomes derived from pending judgments should have status "pending". The aggregator already passes `string(outcome.Status)` into `CoverageOutcome.Status`, so the pending status will flow through automatically once the existing coverage aggregation logic handles it. Check that the switch in `aggregateCoverage` handles pending in its merge logic.

**Files:**
- Modify: `internal/workflow/verification.go:489-524` (aggregateCoverage)
- Test: `internal/workflow/service_test.go`

**Step 1: Write failing test**

Add to `internal/workflow/service_test.go`:

```go
func TestAggregateCoveragePendingStatus(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{
			Status:         domain.JudgmentPending,
			RequirementIDs: []string{"req-1"},
		},
	}
	coverage := aggregateCoverage(outcomes, func(o domain.JudgmentOutcome) []string {
		return o.RequirementIDs
	}, nil, "requirement")
	if len(coverage) != 1 || coverage[0].Status != "pending" {
		t.Fatalf("expected pending coverage, got %+v", coverage)
	}
}

func TestAggregateCoveragePendingDoesNotOverrideFail(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{Status: domain.JudgmentFail, RequirementIDs: []string{"req-1"}},
		{Status: domain.JudgmentPending, RequirementIDs: []string{"req-1"}},
	}
	coverage := aggregateCoverage(outcomes, func(o domain.JudgmentOutcome) []string {
		return o.RequirementIDs
	}, nil, "requirement")
	// Fail should take priority over pending for the same requirement
	if len(coverage) != 1 || coverage[0].Status != "fail" {
		t.Fatalf("expected fail to override pending, got %+v", coverage)
	}
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestAggregateCoverage -v`
Expected: Check whether the current code passes or fails. The existing merge logic uses:
```go
case current == domain.JudgmentFail || outcome.Status == domain.JudgmentFail:
    statuses[id] = domain.JudgmentFail
```
This already handles `fail` overriding `pending` (since `pending` falls to the default case, which sets `JudgmentPass`). The test for pending status may fail because `JudgmentPending` would be set as the initial value but then overridden by the default `JudgmentPass` case on the second iteration. Check and fix if needed.

**Step 3: Write minimal implementation**

Update the merge logic in `aggregateCoverage` to handle pending:

```go
switch {
case !exists:
	statuses[id] = outcome.Status
case current == domain.JudgmentBlocked || outcome.Status == domain.JudgmentBlocked:
	statuses[id] = domain.JudgmentBlocked
case current == domain.JudgmentFail || outcome.Status == domain.JudgmentFail:
	statuses[id] = domain.JudgmentFail
case current == domain.JudgmentPending || outcome.Status == domain.JudgmentPending:
	statuses[id] = domain.JudgmentPending
default:
	statuses[id] = domain.JudgmentPass
}
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestAggregateCoverage -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/workflow/verification.go internal/workflow/service_test.go
git commit -m "feat: aggregateCoverage handles pending status in merge logic"
```

---

### Task 9: Update RunVerification to Persist Pending Attempts

**Objective:** When `RunVerification` produces pending outcomes (rubric judgments), it must persist the attempt with `VerificationPending` verdict and transition the run to `StatusAwaitingJudgment` instead of proceeding to publication/remediation. If no outcomes are pending (all mechanical or BDD), the existing behavior is unchanged.

**Files:**
- Modify: `internal/workflow/verification.go:25-200` (RunVerification)
- Modify: `internal/workflow/verification.go:256-296` (persistVerificationAttempt)
- Test: `internal/workflow/service_test.go`

**Step 1: Write failing test**

The test from Task 6 (`TestVerificationRubricJudgmentReturnsPending`) already asserts `run.Status == StatusAwaitingJudgment`. After Tasks 6-8, the verdict will be `VerificationPending` and the outcome will be pending. The remaining gap is that `persistVerificationAttempt` does not handle the pending verdict — it will fall through to the `default` case (blocked). Run the test:

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestVerificationRubricJudgmentReturnsPending -v`
Expected: FAIL — `persistVerificationAttempt` sets status to `blocked` for unknown verdicts.

**Step 2: Write minimal implementation**

In `internal/workflow/verification.go`, update `persistVerificationAttempt` (line 271) to add a pending case:

```go
switch attempt.Report.Verdict {
case domain.VerificationPass:
	run.Phase = domain.PhaseVerification
	run.Status = domain.StatusInProgress
case domain.VerificationPending:
	run.Phase = domain.PhaseVerification
	run.Status = domain.StatusAwaitingJudgment
case domain.VerificationFail:
	// ... existing fail handling ...
default: // blocked
	run.Phase = domain.PhaseVerification
	run.Status = domain.StatusBlocked
}
```

Also update `RunVerification` at line 182 to skip publication when the verdict is pending:

```go
if verdict == domain.VerificationPass {
	run, err = s.publishPassingGherkin(ctx, execution, run, attempt)
	// ... existing error handling ...
}
```

This already only fires on pass, so no change needed — pending verdicts won't enter this branch.

**Step 3: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestVerificationRubricJudgmentReturnsPending -v`
Expected: PASS

**Step 4: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/workflow/verification.go internal/workflow/service_test.go
git commit -m "feat: persist pending verification attempts with awaiting_judgment status"
```

---

### Task 10: Add JudgeVerification Method to Workflow Service

**Objective:** Create a new `JudgeVerification` method that accepts skill-authored judgments for pending outcomes, validates them, updates the attempt, re-aggregates coverage/verdict, and proceeds with publication/remediation/blocking.

**Files:**
- Modify: `internal/workflow/verification.go` (add JudgeVerification method)
- Test: `internal/workflow/service_test.go`

**Step 1: Write failing test**

Add to `internal/workflow/service_test.go`:

```go
func TestJudgeVerificationCompletesPendingRubric(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "verification-rubric-judge", false)
	completeRubricVerificationCandidate(t, service, execution, profile)

	run, report, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != domain.VerificationPending {
		t.Fatalf("expected pending, got %s", report.Verdict)
	}

	judgments := []domain.SkillJudgment{{
		JudgmentID: "judgment-rubric",
		Status:     domain.JudgmentPass,
		Summary:    "all rubric criteria satisfied",
		Qualitative: &domain.QualitativeJudgment{
			Judge:     "hermoso-verification",
			Reasoning: "command output demonstrates correctness and completeness",
			Criteria:  []string{"correctness", "completeness"},
		},
	}}

	run, report, err = service.JudgeVerification(context.Background(), execution, judgments)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != domain.VerificationPass {
		t.Fatalf("expected pass after judgment, got %s", report.Verdict)
	}
	if run.Status != domain.StatusAwaitingRelease {
		t.Fatalf("expected awaiting_release, got %s", run.Status)
	}
	if run.Publication == nil {
		t.Fatal("expected Gherkin publication after passing judgment")
	}
	if report.Outcomes[0].Status != domain.JudgmentPass {
		t.Fatalf("expected outcome pass, got %s", report.Outcomes[0].Status)
	}
	if report.Outcomes[0].QualitativeJudgment == nil {
		t.Fatal("expected qualitative judgment on outcome")
	}
	if report.Outcomes[0].QualitativeJudgment.Judge != "hermoso-verification" {
		t.Fatalf("expected judge hermoso-verification, got %q",
			report.Outcomes[0].QualitativeJudgment.Judge)
	}
}

func TestJudgeVerificationRejectsJudgmentForNonPendingOutcome(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "verification-rubric-reject", false)
	completeRubricVerificationCandidate(t, service, execution, profile)

	_, _, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}

	// Try to judge a non-existent judgment ID
	judgments := []domain.SkillJudgment{{
		JudgmentID: "nonexistent",
		Status:     domain.JudgmentPass,
		Summary:    "fake",
	}}
	_, _, err = service.JudgeVerification(context.Background(), execution, judgments)
	if err == nil {
		t.Fatal("should reject judgment for unknown judgment ID")
	}
}

func TestJudgeVerificationRejectsMissingJudgments(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "verification-rubric-missing", false)
	completeRubricVerificationCandidate(t, service, execution, profile)

	_, _, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}

	// Submit no judgments — should fail because the pending outcome needs a judgment
	_, _, err = service.JudgeVerification(context.Background(), execution, nil)
	if err == nil {
		t.Fatal("should reject empty judgments when pending outcomes exist")
	}
}

func TestJudgeVerificationRejectsWrongRunState(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "verification-rubric-wrong-state", false)
	completeVerificationCandidate(t, service, execution, profile)

	// Run a BDD (non-rubric) verification — no pending outcomes
	_, _, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}

	// Try to judge — should fail because run is not in awaiting_judgment state
	judgments := []domain.SkillJudgment{{
		JudgmentID: "judgment-feature",
		Status:     domain.JudgmentPass,
		Summary:    "should not be accepted",
	}}
	_, _, err = service.JudgeVerification(context.Background(), execution, judgments)
	if err == nil {
		t.Fatal("should reject judgment when run is not awaiting_judgment")
	}
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestJudgeVerification -v`
Expected: FAIL — `undefined: Service.JudgeVerification`

**Step 3: Write minimal implementation**

Add `JudgeVerification` method to `internal/workflow/verification.go` (after `RunVerification`, before `persistBlockedVerification`):

```go
func (s Service) JudgeVerification(
	ctx context.Context,
	execution domain.ContextRef,
	judgments []domain.SkillJudgment,
) (domain.Run, domain.VerificationReport, error) {
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	if run.Phase != domain.PhaseVerification || run.Status != domain.StatusAwaitingJudgment {
		return domain.Run{}, domain.VerificationReport{}, errors.New("verification judge requires a run in awaiting_judgment state")
	}
	if len(run.VerificationAttempts) == 0 {
		return domain.Run{}, domain.VerificationReport{}, errors.New("no verification attempt to judge")
	}
	attempt := run.VerificationAttempts[len(run.VerificationAttempts)-1]

	// Build a set of pending judgment IDs
	pendingIDs := map[string]int{}
	for i, outcome := range attempt.Report.Outcomes {
		if outcome.Status == domain.JudgmentPending {
			pendingIDs[outcome.JudgmentID] = i
		}
	}
	if len(pendingIDs) == 0 {
		return domain.Run{}, domain.VerificationReport{}, errors.New("no pending outcomes to judge")
	}

	// Validate that every judgment targets a pending outcome and all pending outcomes are covered
	judged := map[string]struct{}{}
	for _, j := range judgments {
		idx, ok := pendingIDs[j.JudgmentID]
		if !ok {
			return domain.Run{}, domain.VerificationReport{}, fmt.Errorf("judgment %q is not a pending outcome", j.JudgmentID)
		}
		if j.Status != domain.JudgmentPass && j.Status != domain.JudgmentFail && j.Status != domain.JudgmentBlocked {
			return domain.Run{}, domain.VerificationReport{}, fmt.Errorf("judgment %q has invalid status %q", j.JudgmentID, j.Status)
		}
		if j.Summary == "" {
			return domain.Run{}, domain.VerificationReport{}, fmt.Errorf("judgment %q must have a summary", j.JudgmentID)
		}
		if _, dup := judged[j.JudgmentID]; dup {
			return domain.Run{}, domain.VerificationReport{}, fmt.Errorf("duplicate judgment for %q", j.JudgmentID)
		}
		judged[j.JudgmentID] = struct{}{}

		// Update the outcome
		outcome := &attempt.Report.Outcomes[idx]
		outcome.Status = j.Status
		outcome.Summary = j.Summary
		if j.Qualitative != nil {
			outcome.QualitativeJudgment = j.Qualitative
		}
	}
	if len(judged) != len(pendingIDs) {
		return domain.Run{}, domain.VerificationReport{}, errors.New("not all pending outcomes have been judged")
	}

	// Re-aggregate coverage and verdict
	attempt.Report.Requirements = aggregateCoverage(attempt.Report.Outcomes, func(o domain.JudgmentOutcome) []string {
		return o.RequirementIDs
	}, run.Design.Verification.CoverageExclusions, "requirement")
	attempt.Report.AcceptanceCriteria = aggregateCoverage(attempt.Report.Outcomes, func(o domain.JudgmentOutcome) []string {
		return o.AcceptanceCriterionIDs
	}, run.Design.Verification.CoverageExclusions, "acceptance_criterion")
	attempt.Report.Verdict = aggregateVerdict(attempt.Report.Outcomes)
	attempt.Report.CompletedAt = s.Now().UTC()

	// Surface resolution: pending outcomes may have been downgraded by missing surfaces.
	// Re-check surface resolutions against the now-finalized outcomes.
	missingSurfaces := map[string]struct{}{}
	for _, resolution := range attempt.Report.SurfaceResolutions {
		if resolution.Status != "resolved" {
			missingSurfaces[resolution.SurfaceID] = struct{}{}
		}
	}
	for i := range attempt.Report.Outcomes {
		for _, surfaceID := range attempt.Report.Outcomes[i].SurfaceIDs {
			if _, missing := missingSurfaces[surfaceID]; missing && attempt.Report.Outcomes[i].Status == domain.JudgmentPass {
				attempt.Report.Outcomes[i].Status = domain.JudgmentFail
				attempt.Report.Outcomes[i].Summary = "candidate interaction surface did not resolve"
			}
		}
	}
	// Re-aggregate after surface downgrade
	attempt.Report.Verdict = aggregateVerdict(attempt.Report.Outcomes)

	if err := validateReportCompleteness(attempt.Report, *run.Design.Verification); err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	reportHash, err := Hash(attempt.Report)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	attempt.ReportHash = reportHash

	run, err = s.completeVerificationAttempt(ctx, execution, attempt)
	if err != nil {
		return domain.Run{}, attempt.Report, err
	}

	if attempt.Report.Verdict == domain.VerificationPass {
		run, err = s.publishPassingGherkin(ctx, execution, run, attempt)
		if err != nil {
			blocked, blockErr := s.Store.UpdateRun(ctx, execution, func(current *domain.Run) error {
				current.Phase = domain.PhaseVerification
				current.Status = domain.StatusBlocked
				current.VerificationBlocker = "Gherkin publication failed: " + err.Error()
				current.Revision++
				current.UpdatedAt = s.Now().UTC()
				return nil
			})
			if blockErr != nil {
				return domain.Run{}, attempt.Report, errors.Join(err, blockErr)
			}
			return blocked, attempt.Report, err
		}
	}
	return run, attempt.Report, err
}
```

Also add `completeVerificationAttempt` (updates an existing attempt rather than appending):

```go
func (s Service) completeVerificationAttempt(
	ctx context.Context,
	execution domain.ContextRef,
	attempt domain.VerificationAttempt,
) (domain.Run, error) {
	return s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if run.Phase != domain.PhaseVerification || run.Status != domain.StatusAwaitingJudgment {
			return errors.New("run is no longer awaiting judgment")
		}
		if len(run.VerificationAttempts) == 0 {
			return errors.New("no verification attempt to complete")
		}
		idx := len(run.VerificationAttempts) - 1
		if run.VerificationAttempts[idx].Number != attempt.Number {
			return errors.New("verification attempt order changed while judging")
		}
		if run.Design == nil || run.Design.PackageHash != attempt.PackageHash {
			return errors.New("design package changed while judging")
		}
		run.VerificationAttempts[idx] = attempt
		now := s.Now().UTC()
		switch attempt.Report.Verdict {
		case domain.VerificationPass:
			run.Phase = domain.PhaseVerification
			run.Status = domain.StatusInProgress
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
				run.Phase = domain.PhaseVerification
				run.Status = domain.StatusBlocked
			}
		default:
			run.Phase = domain.PhaseVerification
			run.Status = domain.StatusBlocked
		}
		run.Revision++
		run.UpdatedAt = now
		return nil
	})
}
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestJudgeVerification -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/workflow/verification.go internal/workflow/service_test.go
git commit -m "feat: add JudgeVerification method for two-phase rubric judgment"
```

---

### Task 11: Add `verification judge` CLI Command

**Objective:** Expose `hermoso verification judge <full-context> <judgments-path>` in the CLI. The command reads a JSON file containing `[]SkillJudgment` and calls `service.JudgeVerification`.

**Files:**
- Modify: `internal/app/app.go:455-495` (runVerificationContract function)
- Test: `internal/app/app_test.go` (or wherever CLI tests live)

**Step 1: Write failing test**

Find the CLI test file and add a test. If app tests use a different pattern, follow it. A simple test:

```go
func TestVerificationJudgeCommand(t *testing.T) {
	// This test would set up a complete rubric verification run,
	// then call the CLI with a judgments JSON file.
	// Follow the pattern of existing CLI tests in the app package.
	t.Skip("TODO: implement CLI test for verification judge — requires full setup")
}
```

Actually, since CLI tests may be complex to set up, focus on the unit test in the workflow layer (Task 10) and add a smoke-level CLI test that verifies the command is wired up:

```go
func TestVerificationJudgeUsage(t *testing.T) {
	// Verify the command rejects missing arguments
	cmd := exec.Command("go", "run", ".", "verification", "judge")
	// This should fail with a usage error
	// ... follow existing CLI test patterns
}
```

**Step 2: Run test to verify failure**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/app/ -run TestVerificationJudge -v`
Expected: FAIL — `verification judge` command does not exist

**Step 3: Write minimal implementation**

In `internal/app/app.go`, update `runVerificationContract` at line 456:

```go
func (a application) runVerificationContract(ctx context.Context, args []string) int {
	if len(args) == 0 || (args[0] != "put" && args[0] != "run" && args[0] != "judge") {
		return a.out.usageError("verification requires: put <full-context> <path>, run <full-context>, or judge <full-context> <judgments-path>")
	}
	action := args[0]
	store, execution, rest, err := a.resolve(ctx, args[1:])
	if err != nil {
		return a.stateFailure(err)
	}
	service, err := a.service(store)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	if action == "run" {
		if len(rest) != 0 {
			return a.out.usageError("verification run does not accept arguments after full context")
		}
		run, report, err := service.RunVerification(ctx, execution)
		if err != nil {
			return a.out.failure(ExitFailure, ErrorState, err.Error())
		}
		return a.out.success(
			"verification run",
			map[string]any{"run": run, "report": report},
			fmt.Sprintf("verification attempt %d: %s\n", report.Attempt, report.Verdict),
		)
	}
	if action == "judge" {
		if len(rest) != 1 {
			return a.out.usageError("verification judge requires exactly one judgments path after full context")
		}
		data, err := a.deps.FS.ReadFile(rest[0])
		if err != nil {
			return a.out.failure(ExitFailure, ErrorInternal, err.Error())
		}
		var judgments []domain.SkillJudgment
		if err := json.Unmarshal(data, &judgments); err != nil {
			return a.out.failure(ExitFailure, ErrorValidation, fmt.Sprintf("decode skill judgments: %v", err))
		}
		run, report, err := service.JudgeVerification(ctx, execution, judgments)
		if err != nil {
			return a.out.failure(ExitFailure, ErrorState, err.Error())
		}
		return a.out.success(
			"verification judge",
			map[string]any{"run": run, "report": report},
			fmt.Sprintf("verification attempt %d judged: %s\n", report.Attempt, report.Verdict),
		)
	}
	// ... existing "put" action ...
```

Also update the usage string at line 48:

```go
hermoso verification <put|run|judge> ...
```

**Step 4: Run test to verify pass**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/app/ -run TestVerificationJudge -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/app/app.go internal/app/app_test.go
git commit -m "feat: add verification judge CLI command"
```

---

### Task 12: Update hermoso-verification SKILL.md for Two-Phase Flow

**Objective:** The verification skill must document the two-phase flow: after `verification run`, check if the report verdict is `pending`. If so, read the pending outcomes and their evidence, apply the rubric criteria, write judgments to a JSON file, and call `verification judge`.

**Files:**
- Modify: `skills/hermoso-verification/SKILL.md`

**Step 1: No test needed (documentation change)**

**Step 2: Write implementation**

Update the `## Execution` section of `skills/hermoso-verification/SKILL.md` to add a new step after `verification run`:

```markdown
### Step 2: Check for Pending Outcomes

After `verification run`, inspect the report verdict:

- **If `pass`**: Proceed to Step 3 (routing — pass).
- **If `fail`**: Proceed to Step 3 (routing — fail/remediation).
- **If `blocked`**: Proceed to Step 3 (routing — blocked).
- **If `pending`**: Rubric outcomes require qualitative judgment. Proceed to Step 2a.

### Step 2a: Apply Rubric Judgments

When the verdict is `pending`:

1. Read the pending outcomes from the report. Each pending outcome has:
   - `judgment_id`: the judgment being evaluated
   - `evidence`: command output (stdout, stderr, exit code, hashes)
   - The contract's rubric policy (judge, criteria) — read from the verification contract

2. For each pending outcome, apply the rubric:
   - Read the command stdout/stderr from the evidence
   - Evaluate against each rubric criterion
   - Form a judgment: pass, fail, or blocked (if evidence is insufficient)
   - Write a reasoned summary explaining the judgment

3. Write the judgments to a JSON file:

```json
[
  {
    "judgment_id": "judgment-rubric-1",
    "status": "pass",
    "summary": "All rubric criteria satisfied. Output demonstrates correctness and completeness.",
    "qualitative": {
      "judge": "hermoso-verification",
      "reasoning": "The command output matches all criteria: ...",
      "criteria": ["correctness", "completeness"]
    }
  }
]
```

4. Call `hermoso verification judge <full-context> <judgments-path> --json`

5. Inspect the result. The verdict will now be `pass`, `fail`, or `blocked`.
   Proceed to Step 3 (routing) based on the final verdict.
```

**Step 3: Verify the skill file is valid**

Run: `cat /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification/skills/hermoso-verification/SKILL.md | head -80`
Expected: The file renders correctly.

**Step 4: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add skills/hermoso-verification/SKILL.md
git commit -m "docs: update hermoso-verification skill for two-phase rubric judgment"
```

---

### Task 13: Add Integration Test for Full Two-Phase Rubric Flow

**Objective:** End-to-end test: set up a rubric verification candidate, run verification (pending), apply skill judgments (pass), verify publication occurs. Also test the fail path: skill judges as fail, remediation round is created.

**Files:**
- Test: `internal/workflow/service_test.go`

**Step 1: Write failing test**

Add to `internal/workflow/service_test.go`:

```go
func TestVerificationRubricFailTriggersRemediation(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "verification-rubric-fail", false)
	completeRubricVerificationCandidate(t, service, execution, profile)

	_, _, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}

	judgments := []domain.SkillJudgment{{
		JudgmentID: "judgment-rubric",
		Status:     domain.JudgmentFail,
		Summary:    "rubric criteria not satisfied: output missing required behavior",
		Qualitative: &domain.QualitativeJudgment{
			Judge:     "hermoso-verification",
			Reasoning: "The command output does not meet the correctness criterion",
			Criteria:  []string{"correctness", "completeness"},
		},
	}}

	run, report, err := service.JudgeVerification(context.Background(), execution, judgments)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != domain.VerificationFail {
		t.Fatalf("expected fail, got %s", report.Verdict)
	}
	if run.Status != domain.StatusPending || run.Phase != domain.PhaseConstruction {
		t.Fatalf("expected construction pending for remediation, got phase=%s status=%s",
			run.Phase, run.Status)
	}
	if len(run.ConstructionRounds) != 2 {
		t.Fatalf("expected remediation round, got %d rounds", len(run.ConstructionRounds))
	}
}
```

**Step 2: Run test to verify pass (should pass after Task 10)**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./internal/workflow/ -run TestVerificationRubricFailTriggersRemediation -v`
Expected: PASS

**Step 3: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add internal/workflow/service_test.go
git commit -m "test: add integration test for rubric fail triggering remediation"
```

---

### Task 14: Run Full Test Suite and Verify No Regressions

**Objective:** Ensure all existing tests still pass with the new pending/judge flow. The key regression risk is that existing BDD verification tests (which use `gherkin` oracle, not `rubric`) should behave exactly as before — no pending state, no awaiting_judgment.

**Files:** None (validation only)

**Step 1: Run full test suite**

Run: `cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification && go test ./... -v 2>&1 | tail -50`
Expected: All tests pass, including:
- `TestVerificationPassPublishesGherkinAndRefreshesModel` (BDD, no pending)
- `TestVerificationRubricJudgmentReturnsPending` (new, rubric pending)
- `TestJudgeVerificationCompletesPendingRubric` (new, judge → pass)
- `TestVerificationRubricFailTriggersRemediation` (new, judge → fail → remediation)
- All existing mutation, blocked, resume tests

**Step 2: If any tests fail, fix them**

Common regression causes:
- `validateReportCompleteness` may reject pending outcomes if the completeness check requires all outcomes to be pass/fail/blocked. Check the function and update if needed.
- `persistVerificationAttempt` may not handle the pending verdict transition correctly. Verify the new case is hit.
- Existing tests that assert `run.Status` after `RunVerification` may need to handle the pending case if they use rubric judgments (unlikely — existing tests use BDD/gherkin).

**Step 3: Commit any fixes**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add -A
git commit -m "fix: resolve test regressions from two-phase verification split"
```

---

### Task 15: Update Critique Document with Resolution

**Objective:** Mark Issue A as resolved in `docs/verification-loop-critique.md` with a summary of the changes made.

**Files:**
- Modify: `docs/verification-loop-critique.md`

**Step 1: Update the document**

Add a resolution note at the top of Issue A:

```markdown
## Issue A: Rubric/Qualitative Oracles Reduced to Exit Codes in Go

**Status: RESOLVED**

**Resolution:** Implemented two-phase verification flow:
- `judgeExecution` now returns `JudgmentPending` for rubric oracle types
- `RunVerification` persists pending attempts with `VerificationPending` verdict and `StatusAwaitingJudgment` run status
- New `JudgeVerification` method accepts skill-authored judgments, validates them, and completes the attempt
- New `verification judge` CLI command exposes the judgment endpoint
- `hermoso-verification` skill updated with the two-phase flow documentation
- Gherkin oracle type remains mechanical (exit code = oracle) — a BDD runner's exit code is deterministic

**Files changed:**
- `internal/domain/verification_report.go` — added JudgmentPending, VerificationPending, QualitativeJudgment, SkillJudgment types
- `internal/domain/lifecycle.go` — added StatusAwaitingJudgment and legal transitions
- `internal/workflow/verification.go` — split judgeExecution, added JudgeVerification and completeVerificationAttempt
- `internal/app/app.go` — added verification judge CLI command
- `skills/hermoso-verification/SKILL.md` — documented two-phase flow
```

**Step 2: Commit**

```bash
cd /home/marcos/Projects/hermoso.worktrees/bdd-contracts-and-verification
git add docs/verification-loop-critique.md
git commit -m "docs: mark Issue A as resolved in critique document"
```

---

## Risks and Tradeoffs

1. **Backward compatibility:** Existing BDD verification runs (gherkin oracle) are unaffected — they never enter the pending state. Only rubric judgments trigger the two-phase flow. Existing tests should pass without modification.

2. **State machine complexity:** Adding `StatusAwaitingJudgment` creates a new branch in the lifecycle. If the skill never calls `verification judge`, the run stays in `awaiting_judgment` indefinitely. This is acceptable — the run can be cancelled or resumed, and the operator can see the pending state via `hermoso status`.

3. **Evidence integrity:** The skill could theoretically submit a judgment that contradicts the evidence (e.g., judging "pass" when stdout shows a failure). Go does not verify the semantic correctness of the skill's judgment — it only validates structural integrity (correct judgment IDs, valid statuses, evidence hashes match). This is by design: the skill is the qualitative judge; Go is the evidence collector and contract enforcer. If the skill produces bad judgments, the fault lies in the skill, not the Go control plane.

4. **Two round-trips for rubric verification:** The two-phase flow requires two CLI invocations (`run` then `judge`) instead of one. This is a minor overhead and is inherent to the skill/Go boundary — Go cannot call the skill, so the skill must be invoked between the two phases.

5. **Gherkin vs rubric distinction:** This fix leaves gherkin as mechanical. If future requirements need qualitative BDD interpretation, a new oracle type (e.g., `gherkin_qualitative`) can be added without changing the existing gherkin flow. This is a deliberate scope decision — Issue A is about rubrics, not BDD.

## Open Questions

1. Should the skill judgment include evidence hashes to prove it read the correct evidence? Currently the validation only checks judgment IDs and statuses. Adding evidence hash verification would make the skill's judgment cryptographically bound to the evidence it evaluated. This is a future enhancement, not required for Issue A.

2. Should there be a timeout on the `awaiting_judgment` state? Currently the run can stay in this state indefinitely. An operator timeout or a cron-based check could auto-cancel stale pending runs. This is an operational concern, not a design issue for this plan.
