package verification

import (
	"strings"
	"testing"
	"time"

	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/domain"
)

var contractTestTime = time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)

func TestValidateContractAcceptsTraceableGherkin(t *testing.T) {
	t.Parallel()
	design, designHash, snapshot, contract, assets := verificationFixture(t)
	if err := ValidateContract(contract, design, designHash, snapshot, assets); err != nil {
		t.Fatal(err)
	}
}

func TestValidateContractRejectsTamperingTagDriftAndMissingCoverage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(*domain.FeatureDesign, *domain.FeatureVerificationContract, map[string][]byte)
		want string
	}{
		{
			name: "tampered asset",
			edit: func(_ *domain.FeatureDesign, _ *domain.FeatureVerificationContract, assets map[string][]byte) {
				assets["features/create.feature"] = append(assets["features/create.feature"], []byte("\n# changed\n")...)
			},
			want: "content hash does not match",
		},
		{
			name: "tag drift",
			edit: func(_ *domain.FeatureDesign, contract *domain.FeatureVerificationContract, _ map[string][]byte) {
				contract.Judgments[0].SurfaceIDs = []string{"surface-other"}
			},
			want: "unknown surface",
		},
		{
			name: "missing coverage",
			edit: func(design *domain.FeatureDesign, _ *domain.FeatureVerificationContract, _ map[string][]byte) {
				design.Requirements = append(design.Requirements, domain.Requirement{
					ID: "req-audit", Title: "Audit", Statement: "Record an audit event.",
					Kind: "functional", Priority: "must",
				})
				design.AcceptanceCriteria = append(design.AcceptanceCriteria, domain.AcceptanceCriterion{
					ID: "ac-audit", Statement: "An audit event is recorded.", RequirementIDs: []string{"req-audit"},
				})
			},
			want: "has no judgment or approved exclusion",
		},
		{
			name: "shell command string",
			edit: func(_ *domain.FeatureDesign, contract *domain.FeatureVerificationContract, _ map[string][]byte) {
				contract.Judgments[0].Execution.Command = []string{"sh", "-c", "go test ./..."}
			},
			want: "execute argv directly",
		},
		{
			name: "covered target exclusion",
			edit: func(_ *domain.FeatureDesign, contract *domain.FeatureVerificationContract, _ map[string][]byte) {
				contract.CoverageExclusions = []domain.CoverageExclusion{{
					TargetKind: "requirement",
					TargetID:   "req-create",
					Rationale:  "Incorrectly excluded despite judgment coverage.",
				}}
			},
			want: "must not exclude a target covered by a judgment",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			design, _, snapshot, contract, assets := verificationFixture(t)
			test.edit(&design, &contract, assets)
			designHash, err := digest.JSON(design)
			if err != nil {
				t.Fatal(err)
			}
			contract.FeatureDesign.Hash = designHash
			if err := ValidateContract(contract, design, designHash, snapshot, assets); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func verificationFixture(t *testing.T) (
	domain.FeatureDesign,
	string,
	domain.ModelSnapshot,
	domain.FeatureVerificationContract,
	map[string][]byte,
) {
	t.Helper()
	context := domain.ContextRef{
		SchemaVersion: domain.ContextSchemaVersion, ProjectID: "project-test",
		FeatureID: "feature-test", RunID: "run-test",
		Repository: domain.TargetIdentity{Repository: "/workspace/project", DefaultBranch: "main"},
	}
	views := map[string]string{
		"index.md": "# Index\n", "views/system-context.md": "# System\n",
		"views/domains.md": "# Domains\n", "views/components.md": "# Components\n",
		"views/runtime.md": "# Runtime\n", "views/data.md": "# Data\n",
		"views/interfaces.md": "# Interfaces\n", "views/testing.md": "# Testing\n",
		"views/operations.md": "# Operations\n", "views/decisions.md": "# Decisions\n",
		"views/vocabulary.md": "# Vocabulary\n",
	}
	sourceHash := digest.Bytes([]byte("source"))
	snapshot, err := domain.FinalizeModelSnapshot(domain.ModelSnapshot{
		Manifest: domain.ModelManifest{
			SchemaVersion: domain.ModelSchemaVersion, ProjectID: context.ProjectID,
			Repository: context.Repository, SourceRevision: strings.Repeat("a", 40),
			VocabularyVersion: "v1", GeneratedAt: contractTestTime,
			Extractors: []domain.ModelExtractor{{Name: "test", Version: "1"}},
		},
		Vocabulary: domain.DefaultModelVocabulary(),
		Nodes: []domain.ModelNode{
			{
				ID: "file:api.go", Kind: "file", Abstraction: "code", Aspects: []string{"structure"},
				Title: "api.go", Summary: "API source.", EpistemicStatus: "observed",
				Evidence: []domain.ModelEvidence{{Path: "api.go", ContentHash: sourceHash}},
				Producer: domain.ModelProducer{Kind: "extractor", Name: "test", Version: "1"},
			},
			{
				ID: "interface:create-api", Kind: "interface", Abstraction: "component", Aspects: []string{"api"},
				Title: "Create API", Summary: "Creates a record.", EpistemicStatus: "derived",
				DerivedFrom: []string{"file:api.go"},
				Producer:    domain.ModelProducer{Kind: "algorithm", Name: "test", Version: "1"},
				Attributes:  map[string]string{"interface_kind": "api"},
			},
			{
				ID: "invariant:unique-name", Kind: "invariant", Abstraction: "component", Aspects: []string{"vocabulary"},
				Title: "Unique name", Summary: "Names are unique.", EpistemicStatus: "stable",
				Evidence: []domain.ModelEvidence{{Path: "api.go", ContentHash: sourceHash}},
				Producer: domain.ModelProducer{Kind: "human", Name: "maintainer", Version: "1"},
			},
		},
		Edges: []domain.ModelEdge{{
			ID: "edge:api", Source: "file:api.go", Relation: "exposes", Target: "interface:create-api",
			EpistemicStatus: "derived", Evidence: []string{"file:api.go"},
			Producer: domain.ModelProducer{Kind: "algorithm", Name: "test", Version: "1"},
		}},
		Views: views,
	})
	if err != nil {
		t.Fatal(err)
	}
	modelReference := domain.ModelReference{
		SchemaVersion: snapshot.Manifest.SchemaVersion, SnapshotID: snapshot.Manifest.SnapshotID,
		ContentHash: snapshot.Manifest.ContentHash, SourceRevision: snapshot.Manifest.SourceRevision,
		VocabularyVersion: snapshot.Manifest.VocabularyVersion,
	}
	design := domain.FeatureDesign{
		SchemaVersion: domain.SchemaVersion, Context: context,
		Producer: domain.Producer{Skill: "hermoso-design", Runtime: "test"}, Revision: 1,
		Feature: domain.FeatureIdentity{
			ID: context.FeatureID, Title: "Create record", Objective: "Create one record",
			TargetRepository: context.Repository,
		},
		BaseModel: modelReference,
		Requirements: []domain.Requirement{{
			ID: "req-create", Title: "Create", Statement: "A user can create a record.",
			Kind: "functional", Priority: "must",
		}},
		AcceptanceCriteria: []domain.AcceptanceCriterion{{
			ID: "ac-created", Statement: "The record is created.", RequirementIDs: []string{"req-create"},
		}},
		BusinessVocabulary: []domain.BusinessTerm{{
			ID: "term-record", Term: "record", Definition: "A stored business record.",
		}},
		Surfaces: []domain.FeatureSurface{{
			ID: "surface-create", Title: "Create API", Kind: "api", Source: "existing",
			ModelNodeID: "interface:create-api", Description: "The record creation endpoint.",
		}},
		UnresolvedQuestions: []string{}, Complexity: domain.ComplexitySmall,
	}
	designHash, err := digest.JSON(design)
	if err != nil {
		t.Fatal(err)
	}
	feature := []byte(`@requirement:req-create @criterion:ac-created @surface:surface-create @term:term-record @invariant:invariant:unique-name
Feature: Create a record

  @scenario:scenario-create @judgment:judgment-create
  Scenario: Create a valid record
    Given no record exists
    When the user creates a record
    Then the record is available
`)
	assets := map[string][]byte{"features/create.feature": feature}
	contract := domain.FeatureVerificationContract{
		SchemaVersion: domain.SchemaVersion, Context: context,
		Producer: domain.Producer{Skill: "hermoso-verification-author", Runtime: "test"},
		Revision: 1,
		FeatureDesign: domain.ContractReference{
			Context: context, Kind: "feature-design", Path: "design.json", Revision: 1, Hash: designHash,
		},
		BaseModel: modelReference,
		Artifacts: []domain.VerificationArtifact{{
			ID: "artifact-create", Kind: "gherkin", Path: "features/create.feature",
			ContentHash: digest.Bytes(feature), PublicationPath: "features/create.feature",
		}},
		Judgments: []domain.VerificationJudgment{{
			ID: "judgment-create", Title: "Create record", Modality: "bdd",
			RequirementIDs: []string{"req-create"}, AcceptanceCriterionIDs: []string{"ac-created"},
			SurfaceIDs: []string{"surface-create"}, BusinessTermIDs: []string{"term-record"},
			InvariantNodeIDs: []string{"invariant:unique-name"},
			ArtifactIDs:      []string{"artifact-create"}, ScenarioIDs: []string{"scenario-create"},
			Execution: domain.VerificationExecution{
				Command: []string{"go", "test", "./..."}, TimeoutSeconds: 300,
			},
			Oracle:           domain.VerificationOracle{Type: "gherkin"},
			RequiredEvidence: []string{"scenario result", "command output"},
		}},
		Aggregation: domain.VerificationAggregation{Strategy: "all_required"},
	}
	return design, designHash, snapshot, contract, assets
}
