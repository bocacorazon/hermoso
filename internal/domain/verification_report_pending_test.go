package domain

import (
	"encoding/json"
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
		ProjectID:  "proj-1",
		FeatureID:  "feat-1",
		RunID:      "run-1",
		Repository: TargetIdentity{Repository: "/workspace/project"},
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
		CandidateModel:     ModelReference{SnapshotID: "snap-1", SourceRevision: "0123456789abcdef0123456789abcdef01234567"},
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
