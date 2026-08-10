package model

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func AddPublishedScenarios(
	snapshot domain.ModelSnapshot,
	design domain.FeatureDesign,
	contract domain.FeatureVerificationContract,
	resolutions []domain.SurfaceResolution,
) (domain.ModelSnapshot, error) {
	nodeIDs := make(map[string]struct{}, len(snapshot.Nodes))
	edgeIDs := make(map[string]struct{}, len(snapshot.Edges))
	for _, node := range snapshot.Nodes {
		nodeIDs[node.ID] = struct{}{}
	}
	for _, edge := range snapshot.Edges {
		edgeIDs[edge.ID] = struct{}{}
	}
	artifacts := make(map[string]domain.VerificationArtifact, len(contract.Artifacts))
	var designEvidence domain.VerificationArtifact
	for _, artifact := range contract.Artifacts {
		artifacts[artifact.ID] = artifact
		if designEvidence.ID == "" && artifact.Kind == "gherkin" {
			designEvidence = artifact
		}
	}
	if designEvidence.ID == "" {
		return domain.ModelSnapshot{}, fmt.Errorf("verification contract has no published Gherkin artifact")
	}
	surfaces := make(map[string]string, len(design.Surfaces))
	for _, surface := range design.Surfaces {
		if surface.Source == "existing" {
			surfaces[surface.ID] = surface.ModelNodeID
		}
	}
	for _, resolution := range resolutions {
		if resolution.Status == "resolved" {
			surfaces[resolution.SurfaceID] = resolution.ModelNodeID
		}
	}
	producer := domain.ModelProducer{Kind: "extractor", Name: "cucumber-gherkin", Version: "v42"}
	designProducerVersion := design.Producer.Version
	if designProducerVersion == "" {
		designProducerVersion = "v2"
	}
	for _, requirement := range design.Requirements {
		id := "requirement:" + requirement.ID
		if _, exists := nodeIDs[id]; exists {
			continue
		}
		snapshot.Nodes = append(snapshot.Nodes, domain.ModelNode{
			ID: id, Kind: "requirement", Abstraction: "component",
			Aspects: []string{"testing"}, Title: requirement.Title, Summary: requirement.Statement,
			EpistemicStatus: "stable", Producer: domain.ModelProducer{
				Kind: "human", Name: design.Producer.Skill, Version: designProducerVersion,
			},
			Evidence: []domain.ModelEvidence{{
				Path: designEvidence.PublicationPath, ContentHash: designEvidence.ContentHash,
			}},
		})
		nodeIDs[id] = struct{}{}
	}
	for _, term := range design.BusinessVocabulary {
		id := "concept:" + term.ID
		if _, exists := nodeIDs[id]; exists {
			continue
		}
		snapshot.Nodes = append(snapshot.Nodes, domain.ModelNode{
			ID: id, Kind: "concept", Abstraction: "component",
			Aspects: []string{"vocabulary"}, Title: term.Term, Summary: term.Definition,
			EpistemicStatus: "stable", Producer: domain.ModelProducer{
				Kind: "human", Name: design.Producer.Skill, Version: designProducerVersion,
			},
			Evidence: []domain.ModelEvidence{{
				Path: designEvidence.PublicationPath, ContentHash: designEvidence.ContentHash,
			}},
		})
		nodeIDs[id] = struct{}{}
	}
	var scenarioLines []string
	for _, judgment := range contract.Judgments {
		if judgment.Modality != "bdd" {
			continue
		}
		var artifact domain.VerificationArtifact
		for _, artifactID := range judgment.ArtifactIDs {
			candidate := artifacts[artifactID]
			if candidate.Kind == "gherkin" {
				artifact = candidate
				break
			}
		}
		if artifact.ID == "" {
			return domain.ModelSnapshot{}, fmt.Errorf("BDD judgment %q has no published Gherkin artifact", judgment.ID)
		}
		fileID := "file:" + artifact.PublicationPath
		if _, ok := nodeIDs[fileID]; !ok {
			return domain.ModelSnapshot{}, fmt.Errorf("published Gherkin file node %q is missing", fileID)
		}
		for _, scenarioID := range judgment.ScenarioIDs {
			nodeID := "scenario:" + scenarioID
			if _, exists := nodeIDs[nodeID]; exists {
				return domain.ModelSnapshot{}, fmt.Errorf("published scenario node %q already exists", nodeID)
			}
			snapshot.Nodes = append(snapshot.Nodes, domain.ModelNode{
				ID: nodeID, Kind: "scenario", Abstraction: "component",
				Aspects: []string{"testing"}, Title: scenarioID,
				Summary:         "Approved and verified business behavior.",
				EpistemicStatus: "stable", DerivedFrom: []string{fileID},
				Evidence: []domain.ModelEvidence{{
					Path: artifact.PublicationPath, ContentHash: artifact.ContentHash,
				}},
				Producer: producer,
			})
			nodeIDs[nodeID] = struct{}{}
			addPublishedEdge(&snapshot, edgeIDs, fileID, "contains", nodeID, fileID, producer)
			for _, requirementID := range judgment.RequirementIDs {
				addPublishedEdge(
					&snapshot, edgeIDs, nodeID, "verifies",
					"requirement:"+requirementID, fileID, producer,
				)
			}
			for _, termID := range judgment.BusinessTermIDs {
				addPublishedEdge(
					&snapshot, edgeIDs, nodeID, "uses_concept",
					"concept:"+termID, fileID, producer,
				)
			}
			for _, surfaceID := range judgment.SurfaceIDs {
				target := surfaces[surfaceID]
				if target == "" {
					return domain.ModelSnapshot{}, fmt.Errorf("scenario %q surface %q is unresolved", scenarioID, surfaceID)
				}
				addPublishedEdge(&snapshot, edgeIDs, nodeID, "exercises", target, fileID, producer)
			}
			for _, invariantID := range judgment.InvariantNodeIDs {
				addPublishedEdge(&snapshot, edgeIDs, nodeID, "governed_by", invariantID, fileID, producer)
			}
			scenarioLines = append(scenarioLines, "- `"+nodeID+"`")
		}
	}
	sort.Strings(scenarioLines)
	if len(scenarioLines) != 0 {
		section := "\n## Published verification scenarios\n\n" + strings.Join(scenarioLines, "\n") + "\n"
		snapshot.Views["index.md"] += section
		snapshot.Views["views/testing.md"] += section
		snapshot.Views["views/vocabulary.md"] += section
	}
	return domain.FinalizeModelSnapshot(snapshot)
}

func addPublishedEdge(
	snapshot *domain.ModelSnapshot,
	ids map[string]struct{},
	source, relation, target, evidence string,
	producer domain.ModelProducer,
) {
	id := "edge:" + source + ":" + relation + ":" + target
	if _, exists := ids[id]; exists {
		return
	}
	snapshot.Edges = append(snapshot.Edges, domain.ModelEdge{
		ID: id, Source: source, Relation: relation, Target: target,
		EpistemicStatus: "stable", Evidence: []string{evidence}, Producer: producer,
	})
	ids[id] = struct{}{}
}
