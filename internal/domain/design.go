package domain

import "fmt"

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

type FeatureDesign struct {
	SchemaVersion       string              `json:"schema_version"`
	Context             ContextRef          `json:"context"`
	Producer            Producer            `json:"producer"`
	Revision            uint64              `json:"revision"`
	Feature             FeatureIdentity     `json:"feature"`
	AcceptanceCriteria  []string            `json:"acceptance_criteria"`
	Constraints         []string            `json:"constraints,omitempty"`
	NonGoals            []string            `json:"non_goals,omitempty"`
	Decisions           []DesignDecision    `json:"decisions,omitempty"`
	UnresolvedQuestions []string            `json:"unresolved_questions"`
	Complexity          Complexity          `json:"complexity"`
	Architecture        []string            `json:"architecture_boundaries,omitempty"`
	Interfaces          []string            `json:"interfaces_and_data_changes,omitempty"`
	Rollout             []string            `json:"rollout_and_compatibility,omitempty"`
	ResearchReferences  []ContractReference `json:"research_references,omitempty"`
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
	validateStringList("acceptance_criteria", d.AcceptanceCriteria, true, &errs)
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
