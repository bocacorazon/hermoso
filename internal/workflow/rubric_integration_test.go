package workflow

import (
	"context"
	"testing"

	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/digest"
)

// putRubricVerificationPackage creates a verification contract with a rubric
// oracle (instead of gherkin), so the outcome will be judgment_pending after
// Phase 1.
func putRubricVerificationPackage(
	t *testing.T,
	service Service,
	execution domain.ContextRef,
	run domain.Run,
) domain.Run {
	t.Helper()
	feature := []byte(`# Rubric verification probe
# Command: go test ./...
# Expected: all tests pass
# Rubric: correctness, completeness
`)
	assets := map[string][]byte{"hidden/probes/feature.txt": feature}
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
			ID: "artifact-feature", Kind: "probe", Path: "hidden/probes/feature.txt",
			ContentHash: digest.Bytes(feature),
		}},
		Judgments: []domain.VerificationJudgment{{
			ID: "judgment-rubric", Title: "Feature works", Modality: "rubric",
			RequirementIDs:         []string{"req-feature"},
			AcceptanceCriterionIDs: []string{"ac-feature"},
			SurfaceIDs:             []string{"surface-feature"},
			ArtifactIDs:            []string{"artifact-feature"},
			Execution: domain.VerificationExecution{
				Command: []string{"go", "test", "./..."}, TimeoutSeconds: 300,
			},
			Oracle:           domain.VerificationOracle{Type: "rubric"},
			RequiredEvidence: []string{"scenario result", "command output"},
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

// completeRubricVerificationCandidate sets up the full lifecycle through
// construction/awaiting_verification, but with a rubric oracle so verification
// run produces a pending outcome instead of a mechanical pass/fail.
func completeRubricVerificationCandidate(
	t *testing.T,
	service Service,
	execution domain.ContextRef,
	profile string,
) domain.Run {
	t.Helper()
	design := testDesign(t, execution, 1, "Verify and publish behavior")
	design.Surfaces[0].Title = "FeatureHandler"
	run, _, err := service.PutDesign(context.Background(), execution, encode(t, design))
	if err != nil {
		t.Fatal(err)
	}
	run = putRubricVerificationPackage(t, service, execution, run)
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
	if _, _, _, err := service.Prepare(context.Background(), execution, profile); err != nil {
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

// TestVerificationRubricPassFlow tests the full two-phase rubric flow:
// Phase 1: verification run → pending (rubric oracle, not evaluated mechanically)
// Phase 2: skill judges as pass → awaiting_release
func TestVerificationRubricPassFlow(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "verification-rubric-pass", false)
	completeRubricVerificationCandidate(t, service, execution, profile)

	_, report, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != domain.VerificationPending {
		t.Fatalf("Phase 1: expected pending verdict, got %s", report.Verdict)
	}

	run, err := service.Store.Run(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.StatusAwaitingJudgment {
		t.Fatalf("Phase 1: expected awaiting_judgment status, got %s", run.Status)
	}

	judgments := []domain.SkillJudgment{{
		JudgmentID: "judgment-rubric",
		Status:     domain.JudgmentPass,
		Summary:    "rubric criteria satisfied: output demonstrates required behavior",
		Qualitative: &domain.QualitativeJudgment{
			Judge:     "hermoso-verification",
			Reasoning: "The command output meets all rubric criteria: correctness, completeness, and behavior alignment with the feature contract.",
			Criteria:  []string{"correctness", "completeness"},
		},
	}}

	run, report, err = service.JudgeVerification(context.Background(), execution, judgments)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != domain.VerificationPass {
		t.Fatalf("Phase 2: expected pass verdict, got %s", report.Verdict)
	}
	if run.Phase != domain.PhaseVerification || run.Status != domain.StatusAwaitingRelease {
		t.Fatalf("Phase 2: expected verification/awaiting_release, got %s/%s",
			run.Phase, run.Status)
	}
}

// TestVerificationRubricFailTriggersRemediation tests that a skill fail
// judgment on the first attempt triggers a remediation round.
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
			Reasoning: "The command output does not meet the correctness criterion.",
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
	if run.Status != domain.StatusAwaitingRemediation || run.Phase != domain.PhaseConstruction {
		t.Fatalf("expected construction awaiting_remediation, got phase=%s status=%s",
			run.Phase, run.Status)
	}
	if len(run.ConstructionRounds) != 1 {
		t.Fatalf("expected no remediation round yet, got %d rounds", len(run.ConstructionRounds))
	}
}
