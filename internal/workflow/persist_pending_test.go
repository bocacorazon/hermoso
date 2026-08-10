package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/digest"
)

func TestPersistVerificationAttemptPendingSetsAwaitingJudgment(t *testing.T) {
	_, service, execution, profile := setupVerificationRun(t, "feature-pending", false)
	run := completeVerificationCandidate(t, service, execution, profile)

	// Build a report with a pending verdict (rubric oracle outcome)
	stdout := []byte("rubric evidence output")
	latest := run.LatestConstruction()
	candidateCommit := latest.IntegratedFeatureCommit
	workspace := latest.Items[0].Workspace
	outcomes := []domain.JudgmentOutcome{{
		JudgmentID:             "judgment-feature",
		Status:                 domain.JudgmentPending,
		RequirementIDs:         []string{"req-feature"},
		AcceptanceCriterionIDs: []string{"ac-feature"},
		SurfaceIDs:             []string{"surface-feature"},
		Summary:                "rubric oracle requires qualitative judgment by the verification skill",
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
		SchemaVersion:    domain.SchemaVersion,
		Context:          execution,
		Producer:         domain.Producer{Skill: "hermoso-verification", Runtime: "deterministic"},
		Attempt:          1,
		CandidateCommit:  candidateCommit,
		DesignPackageHash: run.Design.PackageHash,
		CandidateModel:    run.Design.Feature.BaseModel,
		Outcomes:          outcomes,
		Requirements: aggregateCoverage(outcomes, func(o domain.JudgmentOutcome) []string {
			return o.RequirementIDs
		}, nil, "requirement"),
		AcceptanceCriteria: aggregateCoverage(outcomes, func(o domain.JudgmentOutcome) []string {
			return o.AcceptanceCriterionIDs
		}, nil, "acceptance_criterion"),
		Verdict:     domain.VerificationPending,
		StartedAt:   now,
		CompletedAt: now,
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("report should validate: %v", err)
	}
	reportHash, err := Hash(report)
	if err != nil {
		t.Fatal(err)
	}
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
	if updated.Status != domain.StatusAwaitingJudgment {
		t.Fatalf("pending verdict should set status to awaiting_judgment, got %s", updated.Status)
	}
	if updated.Phase != domain.PhaseVerification {
		t.Fatalf("phase should stay verification, got %s", updated.Phase)
	}
}
