package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/digest"
)

// helper: persist a pending attempt so JudgeVerification has something to judge
func persistPendingAttempt(t *testing.T, service Service, execution domain.ContextRef, profile string) domain.Run {
	t.Helper()
	run := completeRubricVerificationCandidate(t, service, execution, profile)

	latest := run.LatestConstruction()
	candidateCommit := latest.IntegratedFeatureCommit
	workspace := latest.Items[0].Workspace
	stdout := []byte("rubric evidence output")

	outcomes := []domain.JudgmentOutcome{{
		JudgmentID:             "judgment-rubric",
		Status:                 domain.JudgmentPending,
		RequirementIDs:         []string{"req-feature"},
		AcceptanceCriterionIDs: []string{"ac-feature"},
		SurfaceIDs:             []string{"surface-feature"},
		Summary:                "rubric oracle requires qualitative judgment",
		Evidence: domain.VerificationEvidence{
			Command:          []string{"hermoso-hidden-verifier"},
			WorkingDirectory: workspace.Path,
			ExitCode:         0,
			Stdout:           string(stdout),
			StdoutHash:       digest.Bytes(stdout),
			StderrHash:       digest.Bytes(nil),
			DurationMillis:   100,
			Judge:            "rubric",
		},
	}}
	now := testTime.Add(time.Minute)
	report := domain.VerificationReport{
		SchemaVersion:      domain.SchemaVersion,
		Context:            execution,
		Producer:           domain.Producer{Skill: "hermoso-verification", Runtime: "deterministic"},
		Attempt:            1,
		CandidateCommit:    candidateCommit,
		DesignPackageHash:   run.Design.PackageHash,
		CandidateModel:     run.Design.Feature.BaseModel,
		Outcomes:            outcomes,
		Requirements:        aggregateCoverage(outcomes, func(o domain.JudgmentOutcome) []string { return o.RequirementIDs }, nil, "requirement"),
		AcceptanceCriteria:  aggregateCoverage(outcomes, func(o domain.JudgmentOutcome) []string { return o.AcceptanceCriterionIDs }, nil, "acceptance_criterion"),
		Verdict:             domain.VerificationPending,
		StartedAt:           now,
		CompletedAt:         now,
	}
	reportHash, _ := Hash(report)
	attempt := domain.VerificationAttempt{
		Number:           1,
		CandidateCommit:  candidateCommit,
		PackageHash:      run.Design.PackageHash,
		ArtifactRootHash: run.Design.ArtifactRootHash,
		BaseModel:        run.Design.Feature.BaseModel,
		CandidateModel:   run.Design.Feature.BaseModel,
		Workspace:        workspace,
		Report:           report,
		ReportHash:       reportHash,
	}
	updated, err := service.persistVerificationAttempt(context.Background(), execution, attempt)
	if err != nil {
		t.Fatalf("persistVerificationAttempt failed: %v", err)
	}
	return updated
}

func TestJudgeVerificationPassesPendingOutcome(t *testing.T) {
	_, service, execution, profile := setupVerificationRun(t, "feature-judge", false)
	run := persistPendingAttempt(t, service, execution, profile)

	if run.Status != domain.StatusAwaitingJudgment {
		t.Fatalf("precondition: run should be awaiting_judgment, got %s", run.Status)
	}

	judgments := []domain.SkillJudgment{{
		JudgmentID: "judgment-rubric",
		Status:     domain.JudgmentPass,
		Summary:    "all rubric criteria satisfied",
		Qualitative: &domain.QualitativeJudgment{
			Judge:     "rubric",
			Reasoning:  "output demonstrates correct behavior across all criteria",
			Criteria:  []string{"correctness", "completeness"},
		},
	}}

	updated, report, err := service.JudgeVerification(context.Background(), execution, judgments)
	if err != nil {
		t.Fatalf("JudgeVerification failed: %v", err)
	}

	// Outcome should no longer be pending
	if report.Outcomes[0].Status != domain.JudgmentPass {
		t.Fatalf("outcome should be pass after judgment, got %s", report.Outcomes[0].Status)
	}
	if report.Outcomes[0].QualitativeJudgment == nil {
		t.Fatal("qualitative judgment should be populated")
	}

	// Verdict should be pass (all outcomes pass)
	if report.Verdict != domain.VerificationPass {
		t.Fatalf("verdict should be pass, got %s", report.Verdict)
	}

	// Rubric-only contract has no gherkin to publish, so run transitions
	// directly to awaiting_release.
	if updated.Status != domain.StatusAwaitingRelease {
		t.Fatalf("run should be awaiting_release after passing judgment, got %s", updated.Status)
	}
}

func TestJudgeVerificationFailsPendingOutcome(t *testing.T) {
	_, service, execution, profile := setupVerificationRun(t, "feature-judge-fail", false)
	run := persistPendingAttempt(t, service, execution, profile)

	if run.Status != domain.StatusAwaitingJudgment {
		t.Fatalf("precondition: run should be awaiting_judgment, got %s", run.Status)
	}

	judgments := []domain.SkillJudgment{{
		JudgmentID: "judgment-rubric",
		Status:     domain.JudgmentFail,
		Summary:    "rubric criteria not met — output missing required behavior",
		Qualitative: &domain.QualitativeJudgment{
			Judge:     "rubric",
			Reasoning:  "output does not demonstrate correct behavior",
			Criteria:  []string{"correctness"},
		},
	}}

	updated, report, err := service.JudgeVerification(context.Background(), execution, judgments)
	if err != nil {
		t.Fatalf("JudgeVerification failed: %v", err)
	}

	if report.Outcomes[0].Status != domain.JudgmentFail {
		t.Fatalf("outcome should be fail after judgment, got %s", report.Outcomes[0].Status)
	}
	if report.Verdict != domain.VerificationFail {
		t.Fatalf("verdict should be fail, got %s", report.Verdict)
	}

	// First attempt fail should trigger remediation round (construction/pending)
	if updated.Phase != domain.PhaseConstruction {
		t.Fatalf("phase should be construction for remediation, got %s", updated.Phase)
	}
	if updated.Status != domain.StatusPending {
		t.Fatalf("status should be pending for remediation, got %s", updated.Status)
	}
}

func TestJudgeVerificationRejectsJudgmentForNonPendingOutcome(t *testing.T) {
	_, service, execution, profile := setupVerificationRun(t, "feature-judge-reject", false)
	persistPendingAttempt(t, service, execution, profile)

	// Try to judge with an unknown judgment ID
	judgments := []domain.SkillJudgment{{
		JudgmentID: "judgment-nonexistent",
		Status:     domain.JudgmentPass,
		Summary:    "irrelevant",
	}}

	_, _, err := service.JudgeVerification(context.Background(), execution, judgments)
	if err == nil {
		t.Fatal("should reject judgment for unknown judgment ID")
	}
}

func TestJudgeVerificationRejectsIncompleteJudgments(t *testing.T) {
	_, service, execution, profile := setupVerificationRun(t, "feature-judge-incomplete", false)
	persistPendingAttempt(t, service, execution, profile)

	// Submit empty judgments — should fail because there's a pending outcome
	_, _, err := service.JudgeVerification(context.Background(), execution, nil)
	if err == nil {
		t.Fatal("should reject empty judgments when pending outcomes exist")
	}
}
