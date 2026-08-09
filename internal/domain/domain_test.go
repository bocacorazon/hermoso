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
	invalid.SchemaVersion = "2"
	invalid.Context.RunID = "Bad ID"
	invalid.Revision = 0
	invalid.Feature.TargetRepository.Repository = "relative"
	invalid.AcceptanceCriteria = []string{"works", "works", ""}
	invalid.UnresolvedQuestions = []string{"Which API?"}
	invalid.Complexity = "huge"
	err := invalid.Validate()
	assertErrorContains(t, err, "schema_version", "run_id", "revision", "target_repository.repository",
		"acceptance_criteria[1]", "acceptance_criteria[2]", "unresolved_questions", "complexity")
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
			Parents:            []string{"api"},
			Worker:             Worker{Profile: "bad profile", Skills: nil},
		},
		WorkItem{
			ID: "ui", Title: "Cycle", Prompt: "Create a cycle",
			AcceptanceCriteria: []string{"Done"},
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
		ContractHash:  hash,
		Actor:         "developer",
		ApprovedAt:    testTime,
	}
	if err := approval.ValidateContract(PhaseDesign, 3, hash); err != nil {
		t.Fatalf("matching approval: %v", err)
	}
	for name, test := range map[string]func() error{
		"phase":    func() error { return approval.ValidateContract(PhaseConstruction, 3, hash) },
		"revision": func() error { return approval.ValidateContract(PhaseDesign, 4, hash) },
		"hash":     func() error { return approval.ValidateContract(PhaseDesign, 3, "sha256:"+strings.Repeat("b", 64)) },
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
			{Context: testContext(), WorkItemID: "api", TaskID: "task-1", BoundAt: testTime},
			{Context: testContext(), WorkItemID: "api", TaskID: "task-2", BoundAt: testTime},
			{Context: testContext(), WorkItemID: "ui", TaskID: "task-1", BoundAt: testTime},
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
		AcceptanceCriteria:  []string{"The feature works"},
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
			{ID: "api", Title: "API", Prompt: "Build API", AcceptanceCriteria: []string{"API works"}, Worker: worker},
			{ID: "test", Title: "Tests", Prompt: "Test API", AcceptanceCriteria: []string{"Tests pass"}, Parents: []string{"api"}, Worker: worker},
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
