package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/bocacorazon/hermoso/internal/domain"
)

// DeriveVerification synthesizes a FeatureVerificationContract from work-graph
// validation_commands for complexity:small features. Standard/complex features
// are rejected. When bddContract is non-nil, its artifacts and BDD judgments
// are merged into the result. baseModel and designRef provide the contract
// metadata that cannot be derived from the work graph alone.
func (s Service) DeriveVerification(
	_ context.Context,
	execution domain.ContextRef,
	design domain.FeatureDesign,
	graph domain.WorkGraph,
	bddContract *domain.FeatureVerificationContract,
	baseModel domain.ModelReference,
	designRef domain.ContractReference,
) (domain.FeatureVerificationContract, error) {
	if design.Complexity != domain.ComplexitySmall {
		return domain.FeatureVerificationContract{},
			fmt.Errorf("--derive restricted to complexity:small; got %s", design.Complexity)
	}

	contract := domain.FeatureVerificationContract{
		SchemaVersion: "2",
		Context:       execution,
		Producer: domain.Producer{
			Skill:   "hermoso-verification-author",
			Runtime: "automation",
		},
		Revision:                1,
		FeatureDesign:           designRef,
		BaseModel:               baseModel,
		MaxVerificationAttempts: nil, // default 2
		Aggregation: domain.VerificationAggregation{
			Strategy: "all_required",
		},
	}

	// Derive one deterministic exit-code judgment per work item with
	// non-empty validation_commands.
	for _, item := range graph.Items {
		if len(item.ValidationCommands) == 0 {
			continue
		}
		command := strings.Fields(item.ValidationCommands[0])
		if len(command) == 0 {
			continue
		}
		judgment := domain.VerificationJudgment{
			ID:                      fmt.Sprintf("j-derived-%s", item.ID),
			Title:                   fmt.Sprintf("Validation commands for %s", item.ID),
			Modality:                "deterministic",
			RequirementIDs:          item.RequirementIDs,
			AcceptanceCriterionIDs:  item.CriterionIDs,
			SurfaceIDs:              item.SurfaceIDs,
			Execution: domain.VerificationExecution{
				Command:        command,
				TimeoutSeconds: 300,
			},
			Oracle: domain.VerificationOracle{
				Type:     "exit_code",
				Expected: "0",
			},
			RequiredEvidence: []string{"command output", "exit code"},
		}
		contract.Judgments = append(contract.Judgments, judgment)
	}

	// Merge BDD partial contract if provided.
	if bddContract != nil {
		contract.Artifacts = append(contract.Artifacts, bddContract.Artifacts...)
		for _, j := range bddContract.Judgments {
			if j.Modality == "bdd" {
				contract.Judgments = append(contract.Judgments, j)
			}
		}
		if bddContract.MaxVerificationAttempts != nil {
			contract.MaxVerificationAttempts = bddContract.MaxVerificationAttempts
		}
		contract.CoverageExclusions = append(contract.CoverageExclusions, bddContract.CoverageExclusions...)
	}

	if err := contract.Validate(); err != nil {
		return domain.FeatureVerificationContract{}, fmt.Errorf("derived contract validation: %w", err)
	}
	return contract, nil
}