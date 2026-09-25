package workflow

import (
	"testing"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func ctx() domain.ContextRef {
	return domain.ContextRef{
		SchemaVersion: "hermoso-context/v1",
		ProjectID:     "project-0000000000000000000000000a",
		FeatureID:     "test-feature",
		RunID:         "run-00000000000000000000000000000a",
		Repository:    domain.TargetIdentity{Repository: "/tmp/test", RemoteURL: "https://example.com/repo.git", DefaultBranch: "main"},
	}
}

func baseModel() domain.ModelReference {
	return domain.ModelReference{
		SchemaVersion:     "hermoso-repository-model/v1",
		SnapshotID:        "model-0000000000000000000000000000000a",
		ContentHash:       "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		SourceRevision:    "0000000000000000000000000000000000000000",
		VocabularyVersion: "v1",
	}
}

func designRef() domain.ContractReference {
	return domain.ContractReference{
		Context:  ctx(),
		Kind:     "feature-design",
		Path:     "design/feature-design.json",
		Revision: 1,
		Hash:     "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}
}

func gherkinArtifact() domain.VerificationArtifact {
	return domain.VerificationArtifact{
		ID: "artifact-1", Kind: "gherkin",
		Path: "features/test.feature",
		ContentHash:     "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		PublicationPath: "features/test.feature",
	}
}

func TestDeriveVerification_RejectsNonSmall(t *testing.T) {
	var s Service
	design := domain.FeatureDesign{Complexity: domain.ComplexityStandard}
	_, err := s.DeriveVerification(nil, ctx(), design, domain.WorkGraph{}, nil, baseModel(), designRef())
	if err == nil {
		t.Fatal("expected error for standard complexity")
	}
}

func TestDeriveVerification_DerivesFromValidationCommands(t *testing.T) {
	var s Service
	design := domain.FeatureDesign{Complexity: domain.ComplexitySmall}
	graph := domain.WorkGraph{
		SchemaVersion: "2",
		Context:       ctx(),
		Producer:      domain.Producer{Skill: "test", Runtime: "agent"},
		Revision:      1,
		Items: []domain.WorkItem{
			{ID: "item-a", RequirementIDs: []string{"req-1"}, CriterionIDs: []string{"ac-1"}, SurfaceIDs: []string{"surf-1"}, ValidationCommands: []string{"go test ./..."}},
			{ID: "item-b", RequirementIDs: []string{"req-2"}, CriterionIDs: []string{"ac-2"}, SurfaceIDs: []string{"surf-2"}, ValidationCommands: []string{}},
		},
	}

	// Provide a minimal BDD stub with an artifact (validation requires ≥1 artifact).
	bdd := &domain.FeatureVerificationContract{
		SchemaVersion: "2", Context: ctx(), Producer: domain.Producer{Skill: "test", Runtime: "agent"},
		Revision: 1, Artifacts: []domain.VerificationArtifact{gherkinArtifact()},
		Aggregation: domain.VerificationAggregation{Strategy: "all_required"},
	}

	contract, err := s.DeriveVerification(nil, ctx(), design, graph, bdd, baseModel(), designRef())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contract.Judgments) != 1 {
		t.Fatalf("expected 1 judgment, got %d", len(contract.Judgments))
	}
	j := contract.Judgments[0]
	if j.ID != "j-derived-item-a" {
		t.Errorf("expected j-derived-item-a, got %s", j.ID)
	}
	if j.Modality != "deterministic" {
		t.Errorf("expected deterministic, got %s", j.Modality)
	}
	if j.Oracle.Type != "exit_code" || j.Oracle.Expected != "0" {
		t.Errorf("expected exit_code/0 oracle")
	}
}

func TestDeriveVerification_MergesBDDContract(t *testing.T) {
	var s Service
	design := domain.FeatureDesign{Complexity: domain.ComplexitySmall}
	graph := domain.WorkGraph{
		SchemaVersion: "2", Context: ctx(), Producer: domain.Producer{Skill: "test", Runtime: "agent"}, Revision: 1,
		Items: []domain.WorkItem{
			{ID: "item-a", RequirementIDs: []string{"req-1"}, CriterionIDs: []string{"ac-1"}, SurfaceIDs: []string{"surf-1"}, ValidationCommands: []string{"go test ./..."}},
		},
	}

	bdd := &domain.FeatureVerificationContract{
		SchemaVersion: "2", Context: ctx(), Producer: domain.Producer{Skill: "test", Runtime: "agent"}, Revision: 1,
		Artifacts: []domain.VerificationArtifact{gherkinArtifact()},
		Judgments: []domain.VerificationJudgment{
			{ID: "j-bdd-1", Modality: "bdd", Title: "BDD test", RequirementIDs: []string{"req-1"}, AcceptanceCriterionIDs: []string{"ac-1"}, SurfaceIDs: []string{"surf-1"}, ArtifactIDs: []string{"artifact-1"}, RequiredEvidence: []string{"gherkin results"}, ScenarioIDs: []string{"s1"}, Execution: domain.VerificationExecution{Command: []string{"behave"}, TimeoutSeconds: 180}, Oracle: domain.VerificationOracle{Type: "gherkin"}},
		},
		MaxVerificationAttempts: u64p(3),
		Aggregation:             domain.VerificationAggregation{Strategy: "all_required"},
	}

	contract, err := s.DeriveVerification(nil, ctx(), design, graph, bdd, baseModel(), designRef())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contract.Judgments) != 2 {
		t.Fatalf("expected 2 judgments, got %d", len(contract.Judgments))
	}
	if contract.Judgments[0].ID != "j-derived-item-a" {
		t.Errorf("expected j-derived-item-a, got %s", contract.Judgments[0].ID)
	}
	if contract.Judgments[1].ID != "j-bdd-1" {
		t.Errorf("expected j-bdd-1, got %s", contract.Judgments[1].ID)
	}
	if len(contract.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(contract.Artifacts))
	}
	if contract.MaxVerificationAttempts == nil || *contract.MaxVerificationAttempts != 3 {
		t.Errorf("expected max_verification_attempts=3")
	}
}

func u64p(v uint64) *uint64 { return &v }