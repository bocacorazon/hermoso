package verification

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	gherkin "github.com/cucumber/gherkin/go/v42"
	messages "github.com/cucumber/messages/go/v34"

	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/domain"
)

func ValidateContract(
	contract domain.FeatureVerificationContract,
	design domain.FeatureDesign,
	designHash string,
	snapshot domain.ModelSnapshot,
	assets map[string][]byte,
) error {
	if err := contract.Validate(); err != nil {
		return err
	}
	if err := ValidateDesignModel(design, snapshot); err != nil {
		return err
	}
	var problems []string
	if !contract.Context.Equal(design.Context) {
		problems = append(problems, "contract context does not match feature design")
	}
	if contract.FeatureDesign.Revision != design.Revision || contract.FeatureDesign.Hash != designHash {
		problems = append(problems, "feature_design does not bind the exact feature design revision and hash")
	}
	if !sameModelReference(contract.BaseModel, design.BaseModel) {
		problems = append(problems, "contract base_model does not match feature design")
	}
	if !ModelReferenceMatchesSnapshot(design.BaseModel, snapshot) {
		problems = append(problems, "feature design base_model does not match repository model snapshot")
	}

	requirements := map[string]struct{}{}
	for _, requirement := range design.Requirements {
		requirements[requirement.ID] = struct{}{}
	}
	criteria := map[string]struct{}{}
	for _, criterion := range design.AcceptanceCriteria {
		criteria[criterion.ID] = struct{}{}
	}
	terms := map[string]struct{}{}
	for _, term := range design.BusinessVocabulary {
		terms[term.ID] = struct{}{}
	}
	surfaces := map[string]domain.FeatureSurface{}
	for _, surface := range design.Surfaces {
		surfaces[surface.ID] = surface
	}
	modelNodes := map[string]domain.ModelNode{}
	for _, node := range snapshot.Nodes {
		modelNodes[node.ID] = node
	}
	for _, surface := range design.Surfaces {
		if surface.Source != "existing" {
			continue
		}
		node, ok := modelNodes[surface.ModelNodeID]
		if !ok {
			problems = append(problems, fmt.Sprintf("surface %q references unknown model node %q", surface.ID, surface.ModelNodeID))
		} else if node.Kind != "interface" || node.Attributes["interface_kind"] != surface.Kind {
			problems = append(problems, fmt.Sprintf("surface %q does not match model interface kind", surface.ID))
		}
	}

	artifacts := map[string]domain.VerificationArtifact{}
	for _, artifact := range contract.Artifacts {
		artifacts[artifact.ID] = artifact
		data, ok := assets[artifact.Path]
		if !ok {
			problems = append(problems, fmt.Sprintf("artifact %q is missing asset %q", artifact.ID, artifact.Path))
			continue
		}
		if digest.Bytes(data) != artifact.ContentHash {
			problems = append(problems, fmt.Sprintf("artifact %q content hash does not match", artifact.ID))
		}
	}
	for assetPath := range assets {
		found := false
		for _, artifact := range contract.Artifacts {
			if artifact.Path == assetPath {
				found = true
				break
			}
		}
		if !found {
			problems = append(problems, fmt.Sprintf("asset %q is not declared by the contract", assetPath))
		}
	}

	coveredRequirements := map[string]struct{}{}
	coveredCriteria := map[string]struct{}{}
	judgments := map[string]domain.VerificationJudgment{}
	for _, judgment := range contract.Judgments {
		judgments[judgment.ID] = judgment
		for _, id := range judgment.RequirementIDs {
			if _, ok := requirements[id]; !ok {
				problems = append(problems, fmt.Sprintf("judgment %q references unknown requirement %q", judgment.ID, id))
			}
			coveredRequirements[id] = struct{}{}
		}
		for _, id := range judgment.AcceptanceCriterionIDs {
			if _, ok := criteria[id]; !ok {
				problems = append(problems, fmt.Sprintf("judgment %q references unknown acceptance criterion %q", judgment.ID, id))
			}
			coveredCriteria[id] = struct{}{}
		}
		for _, id := range judgment.SurfaceIDs {
			if _, ok := surfaces[id]; !ok {
				problems = append(problems, fmt.Sprintf("judgment %q references unknown surface %q", judgment.ID, id))
			}
		}
		for _, id := range judgment.BusinessTermIDs {
			if _, ok := terms[id]; !ok {
				problems = append(problems, fmt.Sprintf("judgment %q references unknown business term %q", judgment.ID, id))
			}
		}
		for _, id := range judgment.InvariantNodeIDs {
			node, ok := modelNodes[id]
			if !ok || node.Kind != "invariant" {
				problems = append(problems, fmt.Sprintf("judgment %q references unknown invariant %q", judgment.ID, id))
			}
		}
		if judgment.Modality == "bdd" {
			hasGherkin := false
			for _, artifactID := range judgment.ArtifactIDs {
				if artifacts[artifactID].Kind == "gherkin" {
					hasGherkin = true
				}
			}
			if !hasGherkin {
				problems = append(problems, fmt.Sprintf("BDD judgment %q does not reference a Gherkin artifact", judgment.ID))
			}
		} else if len(judgment.ScenarioIDs) != 0 {
			problems = append(problems, fmt.Sprintf("non-BDD judgment %q must not reference scenarios", judgment.ID))
		}
	}

	excludedRequirements := map[string]struct{}{}
	excludedCriteria := map[string]struct{}{}
	for _, exclusion := range contract.CoverageExclusions {
		switch exclusion.TargetKind {
		case "requirement":
			if _, ok := requirements[exclusion.TargetID]; !ok {
				problems = append(problems, fmt.Sprintf("coverage exclusion references unknown requirement %q", exclusion.TargetID))
			}
			if _, covered := coveredRequirements[exclusion.TargetID]; covered {
				problems = append(problems, fmt.Sprintf("requirement %q is both covered and excluded", exclusion.TargetID))
			}
			excludedRequirements[exclusion.TargetID] = struct{}{}
		case "acceptance_criterion":
			if _, ok := criteria[exclusion.TargetID]; !ok {
				problems = append(problems, fmt.Sprintf("coverage exclusion references unknown acceptance criterion %q", exclusion.TargetID))
			}
			if _, covered := coveredCriteria[exclusion.TargetID]; covered {
				problems = append(problems, fmt.Sprintf("acceptance criterion %q is both covered and excluded", exclusion.TargetID))
			}
			excludedCriteria[exclusion.TargetID] = struct{}{}
		}
	}
	for id := range requirements {
		if _, covered := coveredRequirements[id]; !covered {
			if _, excluded := excludedRequirements[id]; !excluded {
				problems = append(problems, fmt.Sprintf("requirement %q has no judgment or approved exclusion", id))
			}
		}
	}
	for id := range criteria {
		if _, covered := coveredCriteria[id]; !covered {
			if _, excluded := excludedCriteria[id]; !excluded {
				problems = append(problems, fmt.Sprintf("acceptance criterion %q has no judgment or approved exclusion", id))
			}
		}
	}

	scenarios := map[string]scenarioRecord{}
	for _, artifact := range contract.Artifacts {
		if artifact.Kind != "gherkin" {
			continue
		}
		parsed, err := parseGherkin(assets[artifact.Path])
		if err != nil {
			problems = append(problems, fmt.Sprintf("artifact %q: %v", artifact.ID, err))
			continue
		}
		for id, scenario := range parsed {
			if _, exists := scenarios[id]; exists {
				problems = append(problems, fmt.Sprintf("scenario ID %q is duplicated across Gherkin artifacts", id))
			}
			scenarios[id] = scenario
		}
	}
	for _, judgment := range contract.Judgments {
		if judgment.Modality != "bdd" {
			continue
		}
		expectedTags := map[string][]string{
			"judgment":    {judgment.ID},
			"requirement": judgment.RequirementIDs,
			"criterion":   judgment.AcceptanceCriterionIDs,
			"surface":     judgment.SurfaceIDs,
			"term":        judgment.BusinessTermIDs,
			"invariant":   judgment.InvariantNodeIDs,
		}
		for _, scenarioID := range judgment.ScenarioIDs {
			scenario, ok := scenarios[scenarioID]
			if !ok {
				problems = append(problems, fmt.Sprintf("judgment %q references unknown Gherkin scenario %q", judgment.ID, scenarioID))
				continue
			}
			for prefix, values := range expectedTags {
				for _, value := range values {
					if !scenario.tags["@"+prefix+":"+value] {
						problems = append(problems, fmt.Sprintf("scenario %q is missing @%s:%s", scenarioID, prefix, value))
					}
				}
			}
		}
	}
	for scenarioID, scenario := range scenarios {
		judgmentIDs := tagValues(scenario.tags, "@judgment:")
		if len(judgmentIDs) != 1 {
			problems = append(problems, fmt.Sprintf("scenario %q must have exactly one @judgment tag", scenarioID))
			continue
		}
		judgment, ok := judgments[judgmentIDs[0]]
		if !ok || judgment.Modality != "bdd" || !contains(judgment.ScenarioIDs, scenarioID) {
			problems = append(problems, fmt.Sprintf("scenario %q is not declared by its BDD judgment", scenarioID))
		}
	}
	if len(problems) != 0 {
		sort.Strings(problems)
		return fmt.Errorf("verification contract cross-validation failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

type scenarioRecord struct {
	tags map[string]bool
}

func parseGherkin(data []byte) (map[string]scenarioRecord, error) {
	upper := strings.ToUpper(string(data))
	for _, placeholder := range []string{"TODO", "TBD", "NEEDS CLARIFICATION", "[PLACEHOLDER]"} {
		if strings.Contains(upper, placeholder) {
			return nil, fmt.Errorf("contains unresolved placeholder %q", placeholder)
		}
	}
	var next uint64
	document, err := gherkin.ParseGherkinDocument(bytes.NewReader(data), func() string {
		return fmt.Sprintf("gherkin-%d", atomic.AddUint64(&next, 1))
	})
	if err != nil {
		return nil, fmt.Errorf("parse Gherkin: %w", err)
	}
	if document.Feature == nil {
		return nil, fmt.Errorf("has no Feature")
	}
	result := map[string]scenarioRecord{}
	featureTags := tagSet(document.Feature.Tags)
	for _, child := range document.Feature.Children {
		if child.Scenario != nil {
			if err := addScenario(result, child.Scenario, featureTags); err != nil {
				return nil, err
			}
		}
		if child.Rule != nil {
			ruleTags := mergeTags(featureTags, tagSet(child.Rule.Tags))
			for _, ruleChild := range child.Rule.Children {
				if ruleChild.Scenario != nil {
					if err := addScenario(result, ruleChild.Scenario, ruleTags); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("contains no scenarios")
	}
	return result, nil
}

func addScenario(result map[string]scenarioRecord, scenario *messages.Scenario, inherited map[string]bool) error {
	tags := mergeTags(inherited, tagSet(scenario.Tags))
	ids := tagValues(tags, "@scenario:")
	if len(ids) != 1 {
		return fmt.Errorf("scenario %q must have exactly one @scenario:<id> tag", scenario.Name)
	}
	if _, exists := result[ids[0]]; exists {
		return fmt.Errorf("scenario ID %q is duplicated", ids[0])
	}
	result[ids[0]] = scenarioRecord{tags: tags}
	return nil
}

func tagSet(tags []*messages.Tag) map[string]bool {
	result := make(map[string]bool, len(tags))
	for _, tag := range tags {
		result[tag.Name] = true
	}
	return result
}

func mergeTags(sets ...map[string]bool) map[string]bool {
	result := map[string]bool{}
	for _, set := range sets {
		for tag := range set {
			result[tag] = true
		}
	}
	return result
}

func tagValues(tags map[string]bool, prefix string) []string {
	var result []string
	for tag := range tags {
		if strings.HasPrefix(tag, prefix) && len(tag) > len(prefix) {
			result = append(result, strings.TrimPrefix(tag, prefix))
		}
	}
	sort.Strings(result)
	return result
}

func sameModelReference(left, right domain.ModelReference) bool {
	return left == right
}

func ModelReferenceMatchesSnapshot(reference domain.ModelReference, snapshot domain.ModelSnapshot) bool {
	return reference.SchemaVersion == snapshot.Manifest.SchemaVersion &&
		reference.SnapshotID == snapshot.Manifest.SnapshotID &&
		reference.ContentHash == snapshot.Manifest.ContentHash &&
		reference.SourceRevision == snapshot.Manifest.SourceRevision &&
		reference.VocabularyVersion == snapshot.Manifest.VocabularyVersion
}

func ValidateDesignModel(design domain.FeatureDesign, snapshot domain.ModelSnapshot) error {
	if err := design.Validate(); err != nil {
		return fmt.Errorf("feature design: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("repository model: %w", err)
	}
	var problems []string
	if snapshot.Manifest.ProjectID != design.Context.ProjectID ||
		!snapshot.Manifest.Repository.Equal(design.Context.Repository) {
		problems = append(problems, "repository model identity does not match feature design context")
	}
	if !ModelReferenceMatchesSnapshot(design.BaseModel, snapshot) {
		problems = append(problems, "feature design base_model does not match repository model snapshot")
	}
	nodes := make(map[string]domain.ModelNode, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		nodes[node.ID] = node
	}
	for _, surface := range design.Surfaces {
		if surface.Source != "existing" {
			continue
		}
		node, ok := nodes[surface.ModelNodeID]
		if !ok {
			problems = append(problems, fmt.Sprintf("surface %q references unknown model node %q", surface.ID, surface.ModelNodeID))
		} else if node.Kind != "interface" || node.Attributes["interface_kind"] != surface.Kind {
			problems = append(problems, fmt.Sprintf("surface %q does not match model interface kind", surface.ID))
		}
	}
	if len(problems) != 0 {
		sort.Strings(problems)
		return fmt.Errorf("feature design model validation failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
