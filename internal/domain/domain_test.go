package domain

import (
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

func TestFeatureDesignValidate(t *testing.T) {
	t.Parallel()

	valid := validDesign()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid design: %v", err)
	}

	invalid := valid
	invalid.SchemaVersion = "1"
	invalid.Context.RunID = "Bad ID"
	invalid.Revision = 0
	invalid.Feature.TargetRepository.Repository = "relative"
	invalid.AcceptanceCriteria = append(invalid.AcceptanceCriteria,
		AcceptanceCriterion{ID: invalid.AcceptanceCriteria[0].ID, Statement: "", RequirementIDs: []string{"missing"}})
	invalid.UnresolvedQuestions = []string{"Which API?"}
	invalid.Complexity = "huge"
	err := invalid.Validate()
	assertErrorContains(t, err, "schema_version", "run_id", "revision", "target_repository.repository",
		"acceptance_criteria[1].id", "acceptance_criteria[1].statement",
		"acceptance_criteria[1].requirement_ids[0]", "unresolved_questions", "complexity")
}

func TestWorkGraphValidateReferencesAndCycle(t *testing.T) {
	t.Parallel()

	valid := validGraph()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid graph: %v", err)
	}

	invalid := valid
	invalid.Items = append(invalid.Items,
		WorkItem{
			ID: "api", Title: "Duplicate", Prompt: "Duplicate ID",
			AcceptanceCriteria: []string{"Done"},
			RequirementIDs:     []string{"req-feature"},
			CriterionIDs:       []string{"ac-feature"},
			SurfaceIDs:         []string{"surface-feature"},
			Parents:            []string{"api"},
			Worker:             Worker{Profile: "bad profile", Skills: nil},
		},
		WorkItem{
			ID: "ui", Title: "Cycle", Prompt: "Create a cycle",
			AcceptanceCriteria: []string{"Done"},
			RequirementIDs:     []string{"req-feature"},
			CriterionIDs:       []string{"ac-feature"},
			SurfaceIDs:         []string{"surface-feature"},
			Parents:            []string{"missing", "test"},
			Worker: Worker{Profile: "default", Skills: []SkillBinding{
				{Name: "tdd"}, {Name: "tdd"},
			}},
		},
	)
	invalid.Items[1].Parents = []string{"ui"}

	err := invalid.Validate()
	assertErrorContains(t, err, "duplicates items[0].id", "must not reference the work item itself",
		"unknown work item", "worker.profile", "worker.skills", "duplicate an earlier skill", "dependency cycle")
}

func TestRunValidateChecksVerificationWithoutConstructionRounds(t *testing.T) {
	t.Parallel()

	run := Run{
		VerificationAttempts: []VerificationAttempt{{
			Number:     1,
			ReportHash: "invalid",
		}},
	}
	err := run.Validate()
	assertErrorContains(t, err, "verification_attempts[0].report_hash")
}

func TestRunValidateChecksVerificationOnceWithMultipleRounds(t *testing.T) {
	t.Parallel()

	run := Run{
		ConstructionRounds: []ConstructionState{{}, {}},
		VerificationAttempts: []VerificationAttempt{{
			Number:     1,
			ReportHash: "invalid",
		}},
	}
	err := run.Validate()
	const target = "verification_attempts[0].report_hash: must use sha256"
	if count := strings.Count(err.Error(), target); count != 1 {
		t.Fatalf("validation error count for %q = %d, want 1: %v", target, count, err)
	}
}

func TestRunValidateRejectsMalformedPublicationAndNonPassingAttempt(t *testing.T) {
	t.Parallel()

	verifiedCommit := strings.Repeat("a", 40)
	publicationCommit := strings.Repeat("b", 40)
	run := Run{
		VerificationAttempts: []VerificationAttempt{{
			Number:          1,
			CandidateCommit: verifiedCommit,
			Report: VerificationReport{
				Verdict: VerificationFail,
			},
		}},
		Publication: &GherkinPublication{
			Attempt:           1,
			VerifiedCommit:    verifiedCommit,
			PublicationCommit: publicationCommit,
			Model: ModelReference{
				ContentHash:    "invalid",
				SourceRevision: publicationCommit,
			},
			PublishedPaths: []string{"features/feature.feature"},
			PublishedAt:    testTime,
		},
	}
	err := run.Validate()
	assertErrorContains(t, err, "gherkin_publication.model.content_hash", "gherkin_publication")
	if !strings.Contains(err.Error(), "must reference a passing verification attempt") {
		t.Fatalf("validation error = %v, want non-passing publication rejection", err)
	}
}

func TestValidateTransition(t *testing.T) {
	t.Parallel()

	valid := []struct {
		fromPhase Phase
		from      RunStatus
		toPhase   Phase
		to        RunStatus
	}{
		{PhaseDesign, StatusPending, PhaseDesign, StatusInProgress},
		{PhaseDesign, StatusAwaitingApproval, PhaseConstruction, StatusPending},
		{PhaseConstruction, StatusInProgress, PhaseConstruction, StatusAwaitingVerification},
		{PhaseConstruction, StatusAwaitingVerification, PhaseVerification, StatusPending},
		{PhaseVerification, StatusAwaitingRelease, PhaseRelease, StatusPending},
		{PhaseRelease, StatusInProgress, PhaseRelease, StatusReleased},
	}
	for _, tc := range valid {
		if err := ValidateTransition(tc.fromPhase, tc.from, tc.toPhase, tc.to); err != nil {
			t.Errorf("%s/%s -> %s/%s: %v", tc.fromPhase, tc.from, tc.toPhase, tc.to, err)
		}
	}

	if err := ValidateTransition(PhaseDesign, StatusPending, PhaseRelease, StatusReleased); err == nil {
		t.Fatal("illegal transition accepted")
	}
	if err := ValidateTransition(PhaseDesign, StatusAwaitingVerification, PhaseDesign, StatusPending); err == nil {
		t.Fatal("invalid source state accepted")
	}
}

func TestApprovalBindsExactRevisionAndHash(t *testing.T) {
	t.Parallel()

	hash := "sha256:" + strings.Repeat("a", 64)
	approval := Approval{
		SchemaVersion: SchemaVersion,
		Context:       testContext(),
		Phase:         PhaseDesign,
		Revision:      3,
		PackageHash:   hash,
		Actor:         "developer",
		ApprovedAt:    testTime,
	}
	if err := approval.ValidatePackage(PhaseDesign, 3, hash); err != nil {
		t.Fatalf("matching approval: %v", err)
	}
	for name, test := range map[string]func() error{
		"phase":    func() error { return approval.ValidatePackage(PhaseConstruction, 3, hash) },
		"revision": func() error { return approval.ValidatePackage(PhaseDesign, 4, hash) },
		"hash":     func() error { return approval.ValidatePackage(PhaseDesign, 3, "sha256:"+strings.Repeat("b", 64)) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := test(); err == nil {
				t.Fatal("mismatch accepted")
			}
		})
	}
}

func TestRunAndPhaseResultValidation(t *testing.T) {
	t.Parallel()

	run := Run{
		SchemaVersion: SchemaVersion, Context: testContext(),
		Phase: PhaseConstruction, Status: StatusInProgress, Revision: 1, CreatedAt: testTime, UpdatedAt: testTime,
		TaskBindings: []TaskBinding{
			{Context: testContext(), Round: 1, WorkItemID: "api", TaskID: "task-1", BoundAt: testTime},
			{Context: testContext(), Round: 1, WorkItemID: "api", TaskID: "task-2", BoundAt: testTime},
			{Context: testContext(), Round: 1, WorkItemID: "ui", TaskID: "task-1", BoundAt: testTime},
		},
	}
	assertErrorContains(t, run.Validate(), "more than one task binding", "more than one work item")

	result := PhaseResult{
		SchemaVersion: SchemaVersion, Context: testContext(), Producer: Producer{Skill: "design", Runtime: "hermes"},
		Phase: PhaseDesign, Status: ResultBlocked, Summary: "Needs input", CompletedAt: testTime,
	}
	assertErrorContains(t, result.Validate(), "must contain at least one blocker")
	result.Blockers = []string{"API is undecided"}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid blocked result: %v", err)
	}
	result.Status = ResultCompleted
	assertErrorContains(t, result.Validate(), "must be empty when status is completed")
}

func validDesign() FeatureDesign {
	return FeatureDesign{
		SchemaVersion: SchemaVersion,
		Context:       testContext(),
		Producer:      Producer{Skill: "hermoso-design", Runtime: "hermes"},
		Revision:      1,
		Feature: FeatureIdentity{
			ID: "feature-1", Title: "Feature", Objective: "Deliver the feature",
			TargetRepository: TargetIdentity{Repository: "/workspace/project"},
		},
		BaseModel: ModelReference{
			SchemaVersion:     ModelSchemaVersion,
			SnapshotID:        "model-" + strings.Repeat("a", 32),
			ContentHash:       "sha256:" + strings.Repeat("b", 64),
			SourceRevision:    strings.Repeat("c", 40),
			VocabularyVersion: "v1",
		},
		Requirements: []Requirement{{
			ID: "req-feature", Title: "Feature works", Statement: "The feature works.",
			Kind: "functional", Priority: "must",
		}},
		AcceptanceCriteria: []AcceptanceCriterion{{
			ID: "ac-feature", Statement: "The feature works.", RequirementIDs: []string{"req-feature"},
		}},
		BusinessVocabulary: []BusinessTerm{{
			ID: "term-feature", Term: "feature", Definition: "The requested capability.",
		}},
		Surfaces: []FeatureSurface{{
			ID: "surface-feature", Title: "Feature API", Kind: "api", Source: "planned",
			Description: "The feature interaction surface.",
		}},
		UnresolvedQuestions: []string{},
		Complexity:          ComplexitySmall,
	}
}

func validGraph() WorkGraph {
	worker := Worker{Profile: "default", Skills: []SkillBinding{{Name: "test-driven-development"}}}
	return WorkGraph{
		SchemaVersion: SchemaVersion, Context: testContext(),
		Producer: Producer{Skill: "hermoso-design", Runtime: "hermes"}, Revision: 1,
		Items: []WorkItem{
			{
				ID: "api", Title: "API", Prompt: "Build API", AcceptanceCriteria: []string{"API works"},
				RequirementIDs: []string{"req-feature"}, CriterionIDs: []string{"ac-feature"},
				SurfaceIDs: []string{"surface-feature"}, Worker: worker,
			},
			{
				ID: "test", Title: "Tests", Prompt: "Test API", AcceptanceCriteria: []string{"Tests pass"},
				RequirementIDs: []string{"req-feature"}, CriterionIDs: []string{"ac-feature"},
				SurfaceIDs: []string{"surface-feature"}, Parents: []string{"api"}, Worker: worker,
			},
		},
	}

}

func testContext() ContextRef {
	return ContextRef{
		SchemaVersion: ContextSchemaVersion,
		ProjectID:     "project-1",
		FeatureID:     "feature-1",
		RunID:         "run-1",
		Repository:    TargetIdentity{Repository: "/workspace/project"},
	}
}

func assertErrorContains(t *testing.T, err error, values ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("validation unexpectedly succeeded")
	}
	for _, value := range values {
		if !strings.Contains(err.Error(), value) {
			t.Errorf("error %q does not contain %q", err, value)
		}
	}
}
