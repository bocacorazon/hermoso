package domain

import (
	"fmt"
	"strings"
)

type Complexity string

const (
	ComplexitySmall    Complexity = "small"
	ComplexityStandard Complexity = "standard"
	ComplexityComplex  Complexity = "complex"
)

type FeatureIdentity struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Objective        string         `json:"objective"`
	TargetRepository TargetIdentity `json:"target_repository"`
}

type DesignDecision struct {
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
}

type Requirement struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Statement string `json:"statement"`
	Kind      string `json:"kind"`
	Priority  string `json:"priority"`
	Rationale string `json:"rationale,omitempty"`
}

type AcceptanceCriterion struct {
	ID             string   `json:"id"`
	Statement      string   `json:"statement"`
	RequirementIDs []string `json:"requirement_ids"`
}

type BusinessTerm struct {
	ID         string   `json:"id"`
	Term       string   `json:"term"`
	Definition string   `json:"definition"`
	Aliases    []string `json:"aliases,omitempty"`
}

type ModelReference struct {
	SchemaVersion     string `json:"schema_version"`
	SnapshotID        string `json:"snapshot_id"`
	ContentHash       string `json:"content_hash"`
	SourceRevision    string `json:"source_revision"`
	VocabularyVersion string `json:"vocabulary_version"`
}

type FeatureSurface struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	ModelNodeID string `json:"model_node_id,omitempty"`
	Description string `json:"description"`
}

type FeatureDesign struct {
	SchemaVersion       string                `json:"schema_version"`
	Context             ContextRef            `json:"context"`
	Producer            Producer              `json:"producer"`
	Revision            uint64                `json:"revision"`
	Feature             FeatureIdentity       `json:"feature"`
	BaseModel           ModelReference        `json:"base_model"`
	Requirements        []Requirement         `json:"requirements"`
	AcceptanceCriteria  []AcceptanceCriterion `json:"acceptance_criteria"`
	BusinessVocabulary  []BusinessTerm        `json:"business_vocabulary,omitempty"`
	Surfaces            []FeatureSurface      `json:"interaction_surfaces"`
	Constraints         []string              `json:"constraints,omitempty"`
	NonGoals            []string              `json:"non_goals,omitempty"`
	Decisions           []DesignDecision      `json:"decisions,omitempty"`
	UnresolvedQuestions []string              `json:"unresolved_questions"`
	Complexity          Complexity            `json:"complexity"`
	Architecture        []string              `json:"architecture_boundaries,omitempty"`
	Interfaces          []string              `json:"interfaces_and_data_changes,omitempty"`
	Rollout             []string              `json:"rollout_and_compatibility,omitempty"`
	ResearchReferences  []ContractReference   `json:"research_references,omitempty"`
}

func (d FeatureDesign) Validate() error {
	var errs ValidationErrors
	validateVersion(d.SchemaVersion, &errs)
	d.Context.validate("context", &errs)
	d.Producer.validate("producer", &errs)
	if d.Revision == 0 {
		errs.add("revision", "must be greater than zero")
	}
	validateID("feature.id", d.Feature.ID, &errs)
	if d.Feature.ID != d.Context.FeatureID {
		errs.add("feature.id", "must match context.feature_id")
	}
	validateRequired("feature.title", d.Feature.Title, &errs)
	validateRequired("feature.objective", d.Feature.Objective, &errs)
	d.Feature.TargetRepository.validate("feature.target_repository", &errs)
	if !d.Feature.TargetRepository.Equal(d.Context.Repository) {
		errs.add("feature.target_repository", "must match context.repository")
	}
	d.BaseModel.validate("base_model", &errs)
	requirements := map[string]struct{}{}
	for i, requirement := range d.Requirements {
		path := fmt.Sprintf("requirements[%d]", i)
		validateID(path+".id", requirement.ID, &errs)
		if _, ok := requirements[requirement.ID]; ok {
			errs.add(path+".id", "must be unique")
		}
		requirements[requirement.ID] = struct{}{}
		validateRequired(path+".title", requirement.Title, &errs)
		validateRequired(path+".statement", requirement.Statement, &errs)
		if requirement.Kind != "functional" && requirement.Kind != "non_functional" &&
			requirement.Kind != "security" && requirement.Kind != "compatibility" &&
			requirement.Kind != "structural" {
			errs.add(path+".kind", "must be functional, non_functional, security, compatibility, or structural")
		}
		if requirement.Priority != "must" && requirement.Priority != "should" && requirement.Priority != "could" {
			errs.add(path+".priority", "must be must, should, or could")
		}
	}
	if len(d.Requirements) == 0 {
		errs.add("requirements", "must contain at least one requirement")
	}
	criteria := map[string]struct{}{}
	coveredRequirements := map[string]struct{}{}
	for i, criterion := range d.AcceptanceCriteria {
		path := fmt.Sprintf("acceptance_criteria[%d]", i)
		validateID(path+".id", criterion.ID, &errs)
		if _, ok := criteria[criterion.ID]; ok {
			errs.add(path+".id", "must be unique")
		}
		criteria[criterion.ID] = struct{}{}
		validateRequired(path+".statement", criterion.Statement, &errs)
		if len(criterion.RequirementIDs) == 0 {
			errs.add(path+".requirement_ids", "must contain at least one requirement ID")
		}
		seen := map[string]struct{}{}
		for j, requirementID := range criterion.RequirementIDs {
			refPath := fmt.Sprintf("%s.requirement_ids[%d]", path, j)
			validateID(refPath, requirementID, &errs)
			if _, ok := requirements[requirementID]; !ok {
				errs.add(refPath, "references an unknown requirement")
			}
			if _, ok := seen[requirementID]; ok {
				errs.add(refPath, "must be unique")
			}
			seen[requirementID] = struct{}{}
			coveredRequirements[requirementID] = struct{}{}
		}
	}
	if len(d.AcceptanceCriteria) == 0 {
		errs.add("acceptance_criteria", "must contain at least one criterion")
	}
	for requirementID := range requirements {
		if _, ok := coveredRequirements[requirementID]; !ok {
			errs.add("requirements", fmt.Sprintf("requirement %q has no acceptance criterion", requirementID))
		}
	}
	terms := map[string]struct{}{}
	words := map[string]string{}
	for i, term := range d.BusinessVocabulary {
		path := fmt.Sprintf("business_vocabulary[%d]", i)
		validateID(path+".id", term.ID, &errs)
		if _, ok := terms[term.ID]; ok {
			errs.add(path+".id", "must be unique")
		}
		terms[term.ID] = struct{}{}
		validateRequired(path+".term", term.Term, &errs)
		validateRequired(path+".definition", term.Definition, &errs)
		for j, word := range append([]string{term.Term}, term.Aliases...) {
			wordPath := path + ".term"
			if j > 0 {
				wordPath = fmt.Sprintf("%s.aliases[%d]", path, j-1)
			}
			validateRequired(wordPath, word, &errs)
			key := strings.ToLower(strings.TrimSpace(word))
			if prior, ok := words[key]; ok && prior != term.ID {
				errs.add(wordPath, "duplicates a term or alias from "+prior)
			}
			words[key] = term.ID
		}
	}
	surfaces := map[string]struct{}{}
	for i, surface := range d.Surfaces {
		path := fmt.Sprintf("interaction_surfaces[%d]", i)
		validateID(path+".id", surface.ID, &errs)
		if _, ok := surfaces[surface.ID]; ok {
			errs.add(path+".id", "must be unique")
		}
		surfaces[surface.ID] = struct{}{}
		validateRequired(path+".title", surface.Title, &errs)
		validateRequired(path+".description", surface.Description, &errs)
		if surface.Kind != "api" && surface.Kind != "cli" && surface.Kind != "ui" &&
			surface.Kind != "file" && surface.Kind != "event" && surface.Kind != "library" {
			errs.add(path+".kind", "must be api, cli, ui, file, event, or library")
		}
		if surface.Source != "existing" && surface.Source != "planned" {
			errs.add(path+".source", "must be existing or planned")
		}
		if surface.Source == "existing" {
			validateModelIdentifier(path+".model_node_id", surface.ModelNodeID, &errs)
		} else if surface.ModelNodeID != "" {
			errs.add(path+".model_node_id", "must be empty for a planned surface")
		}
	}
	if len(d.Surfaces) == 0 {
		errs.add("interaction_surfaces", "must contain at least one surface")
	}
	validateStringList("constraints", d.Constraints, false, &errs)
	validateStringList("non_goals", d.NonGoals, false, &errs)
	validateStringList("unresolved_questions", d.UnresolvedQuestions, false, &errs)
	if len(d.UnresolvedQuestions) != 0 {
		errs.add("unresolved_questions", "must be empty before the design can be accepted")
	}
	if d.Complexity != ComplexitySmall && d.Complexity != ComplexityStandard && d.Complexity != ComplexityComplex {
		errs.add("complexity", "must be small, standard, or complex")
	}
	for i, decision := range d.Decisions {
		validateRequired(fmt.Sprintf("decisions[%d].decision", i), decision.Decision, &errs)
		validateRequired(fmt.Sprintf("decisions[%d].rationale", i), decision.Rationale, &errs)
	}
	for i, ref := range d.ResearchReferences {
		ref.validate(fmt.Sprintf("research_references[%d]", i), &errs)
		if !ref.Context.Equal(d.Context) {
			errs.add(fmt.Sprintf("research_references[%d].context", i), "must match feature design context")
		}
	}
	return validationResult(errs)
}

func (r ModelReference) validate(path string, errs *ValidationErrors) {
	if r.SchemaVersion != ModelSchemaVersion {
		errs.add(path+".schema_version", "must be "+ModelSchemaVersion)
	}
	if !strings.HasPrefix(r.SnapshotID, "model-") || len(r.SnapshotID) != 38 {
		errs.add(path+".snapshot_id", "must identify a repository model snapshot")
	}
	if !hashPattern.MatchString(r.ContentHash) {
		errs.add(path+".content_hash", "must use sha256:<64 lowercase hex characters>")
	}
	if !gitObjectPattern.MatchString(r.SourceRevision) {
		errs.add(path+".source_revision", "must be a full lowercase Git object ID")
	}
	validateRequired(path+".vocabulary_version", r.VocabularyVersion, errs)
}
