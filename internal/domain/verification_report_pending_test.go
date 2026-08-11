package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

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

func testContextRef() ContextRef {
	return ContextRef{
		SchemaVersion: ContextSchemaVersion,
		ProjectID:     "proj-1",
		FeatureID:     "feat-1",
		RunID:         "run-1",
		Repository:    TargetIdentity{Repository: "/workspace/project"},
	}
}

// validPendingReport is used by validation tests in subsequent tasks.
func validPendingReport() VerificationReport {
	return VerificationReport{
		SchemaVersion:      SchemaVersion,
		Context:            testContextRef(),
		Producer:           Producer{Skill: "hermoso-verification", Runtime: "deterministic"},
		Attempt:            1,
		CandidateCommit:    "0123456789abcdef0123456789abcdef01234567",
		DesignPackageHash:  "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CandidateModel: ModelReference{
			SchemaVersion:     ModelSchemaVersion,
			SnapshotID:        "model-" + strings.Repeat("a", 32),
			ContentHash:       "sha256:" + strings.Repeat("b", 64),
			SourceRevision:    "0123456789abcdef0123456789abcdef01234567",
			VocabularyVersion: "v1",
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

// Suppress unused import warnings — these will be used in later tasks.
var _ = json.Marshal
var _ = time.Now

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
			Judge:      "hermoso-verification",
			Reasoning:  "output matches criteria",
			Criteria:   []string{"correctness"},
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

func TestReportValidatePassRejectsPendingOutcome(t *testing.T) {
	report := validPendingReport()
	report.Verdict = VerificationPass
	if err := report.Validate(); err == nil {
		t.Fatal("pass verdict with pending outcome should fail")
	}
}

func TestReportValidateFailRejectsPendingOutcome(t *testing.T) {
	report := validPendingReport()
	// Keep the pending outcome, try to set verdict to fail
	report.Verdict = VerificationFail
	if err := report.Validate(); err == nil {
		t.Fatal("fail verdict should not allow pending outcomes")
	}
}
