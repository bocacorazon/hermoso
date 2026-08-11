package workflow

import (
	"context"
	"testing"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func TestPutRemediationRejectsWrongState(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "put-remediation-wrong", true)
	completeVerificationCandidate(t, service, execution, profile)

	// Run is in construction/awaiting_verification, not awaiting_remediation.
	spec := domain.SkillRemediation{
		Needs: []domain.RemediationNeed{{
			RequirementIDs: []string{"req-feature"},
			Expected:       "x",
			Actual:         "y",
		}},
		Graph: domain.WorkGraph{
			SchemaVersion: domain.SchemaVersion,
			Context:       execution,
			Producer:      domain.Producer{Skill: "hermoso-verification", Runtime: "agent"},
			Revision:      1,
			Items: []domain.WorkItem{{
				ID:             "r-1",
				Title:          "fix",
				Prompt:         "fix it",
				RequirementIDs: []string{"req-feature"},
				SurfaceIDs:     []string{"surface-feature"},
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
	run, err = resolvePendingSurfaces(t, service, execution)
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
				ID:         "r-1",
				Title:      "fix",
				Prompt:     "fix it",
				SurfaceIDs: []string{"surface-feature"},
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

func TestPutRemediationRejectsContextMismatch(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "put-remediation-ctx", true)
	completeVerificationCandidate(t, service, execution, profile)

	run, _, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	run, err = resolvePendingSurfaces(t, service, execution)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.StatusAwaitingRemediation {
		t.Fatalf("expected awaiting_remediation, got %s", run.Status)
	}

	// Graph context doesn't match the run context.
	wrongCtx := execution
	wrongCtx.RunID = "different-run"
	spec := domain.SkillRemediation{
		Needs: []domain.RemediationNeed{{
			RequirementIDs: []string{"req-feature"},
			Expected:       "x",
			Actual:         "y",
		}},
		Graph: domain.WorkGraph{
			SchemaVersion: domain.SchemaVersion,
			Context:       wrongCtx,
			Producer:      domain.Producer{Skill: "hermoso-verification", Runtime: "agent"},
			Revision:      1,
			Items: []domain.WorkItem{{
				ID:             "r-1",
				Title:          "fix",
				Prompt:         "fix it",
				RequirementIDs: []string{"req-feature"},
				SurfaceIDs:     []string{"surface-feature"},
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
		t.Fatal("expected error for context mismatch")
	}
}

// TestRemediationFullFlow verifies the complete two-phase remediation lifecycle:
// verification fail → awaiting_remediation → put remediation → construction → second verification → pass.
// The TestVerificationFailureCreatesOneRemediationRound test already covers this
// end-to-end, so this test focuses on verifying the remediation round's
// deterministic fields are correctly set by Go.
func TestRemediationRoundDeterministicFields(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setupVerificationRun(t, "remediation-fields", true)
	completeVerificationCandidate(t, service, execution, profile)

	run, _, err := service.RunVerification(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	run, err = resolvePendingSurfaces(t, service, execution)
	if err != nil {
		t.Fatal(err)
	}

	run, err = putRemediationSpec(t, service, execution)
	if err != nil {
		t.Fatal(err)
	}

	remediation := run.LatestConstruction()
	if remediation == nil {
		t.Fatal("expected remediation round")
	}
	if remediation.Kind != "remediation" {
		t.Errorf("expected kind=remediation, got %s", remediation.Kind)
	}
	if remediation.Number != 2 {
		t.Errorf("expected number=2 (second construction round), got %d", remediation.Number)
	}
	if remediation.SourceHash != run.VerificationAttempts[0].ReportHash {
		t.Errorf("SourceHash should match failed report hash")
	}
	if remediation.Remediation == nil {
		t.Fatal("expected non-nil RemediationSpec")
	}
	if remediation.Remediation.FailedReportHash != run.VerificationAttempts[0].ReportHash {
		t.Errorf("FailedReportHash should match failed report hash")
	}
	if remediation.Remediation.SchemaVersion != domain.SchemaVersion {
		t.Errorf("expected schema_version=%s, got %s", domain.SchemaVersion, remediation.Remediation.SchemaVersion)
	}
	if !remediation.Remediation.Context.Equal(execution) {
		t.Errorf("remediation spec context should match execution context")
	}
	if remediation.Graph.Producer.Runtime != "agent" {
		t.Errorf("expected agent runtime, got %s", remediation.Graph.Producer.Runtime)
	}
	if remediation.Hash == "" {
		t.Error("expected non-empty graph hash")
	}
}
