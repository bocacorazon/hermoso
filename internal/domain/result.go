package domain

import (
	"fmt"
	"time"
)

type ResultStatus string

const (
	ResultCompleted ResultStatus = "completed"
	ResultBlocked   ResultStatus = "blocked"
	ResultFailed    ResultStatus = "failed"
	ResultCancelled ResultStatus = "cancelled"
)

type ContractReference struct {
	Context  ContextRef `json:"context"`
	Kind     string     `json:"kind"`
	Path     string     `json:"path"`
	Revision uint64     `json:"revision"`
	Hash     string     `json:"hash"`
}

func (r ContractReference) validate(path string, errs *ValidationErrors) {
	r.Context.validate(path+".context", errs)
	validateRequired(path+".kind", r.Kind, errs)
	validateRequired(path+".path", r.Path, errs)
	if r.Revision == 0 {
		errs.add(path+".revision", "must be greater than zero")
	}
	if !hashPattern.MatchString(r.Hash) {
		errs.add(path+".hash", "must use sha256:<64 lowercase hex characters>")
	}
}

type PhaseResult struct {
	SchemaVersion string              `json:"schema_version"`
	Context       ContextRef          `json:"context"`
	Producer      Producer            `json:"producer"`
	Phase         Phase               `json:"phase"`
	Status        ResultStatus        `json:"status"`
	Inputs        []ContractReference `json:"inputs,omitempty"`
	Outputs       []ContractReference `json:"outputs,omitempty"`
	Summary       string              `json:"summary"`
	Decisions     []string            `json:"decisions,omitempty"`
	Warnings      []string            `json:"warnings,omitempty"`
	Blockers      []string            `json:"unresolved_blockers,omitempty"`
	Evidence      []Evidence          `json:"evidence,omitempty"`
	CompletedAt   time.Time           `json:"completed_at"`
}

func (r PhaseResult) Validate() error {
	var errs ValidationErrors
	validateVersion(r.SchemaVersion, &errs)
	r.Context.validate("context", &errs)
	r.Producer.validate("producer", &errs)
	if !r.Phase.valid() {
		errs.add("phase", "is not a recognized phase")
	}
	if r.Status != ResultCompleted && r.Status != ResultBlocked && r.Status != ResultFailed && r.Status != ResultCancelled {
		errs.add("status", "must be completed, blocked, failed, or cancelled")
	}
	if r.Status == ResultBlocked && len(r.Blockers) == 0 {
		errs.add("unresolved_blockers", "must contain at least one blocker when status is blocked")
	}
	if r.Status == ResultCompleted && len(r.Blockers) != 0 {
		errs.add("unresolved_blockers", "must be empty when status is completed")
	}
	validateRequired("summary", r.Summary, &errs)
	validateStringList("decisions", r.Decisions, false, &errs)
	validateStringList("warnings", r.Warnings, false, &errs)
	validateStringList("unresolved_blockers", r.Blockers, false, &errs)
	for i, ref := range r.Inputs {
		ref.validate(fmt.Sprintf("inputs[%d]", i), &errs)
		if !ref.Context.Equal(r.Context) {
			errs.add(fmt.Sprintf("inputs[%d].context", i), "must match phase result context")
		}
	}
	for i, ref := range r.Outputs {
		ref.validate(fmt.Sprintf("outputs[%d]", i), &errs)
		if !ref.Context.Equal(r.Context) {
			errs.add(fmt.Sprintf("outputs[%d].context", i), "must match phase result context")
		}
	}
	evidenceIDs := make(map[string]struct{}, len(r.Evidence))
	for i, item := range r.Evidence {
		path := fmt.Sprintf("evidence[%d]", i)
		item.validate(path, &errs)
		if !item.Context.Equal(r.Context) {
			errs.add(path+".context", "must match phase result context")
		}
		if _, ok := evidenceIDs[item.ID]; ok {
			errs.add(path+".id", "must be unique")
		}
		evidenceIDs[item.ID] = struct{}{}
	}
	if r.CompletedAt.IsZero() {
		errs.add("completed_at", "must be set")
	}
	return validationResult(errs)
}

type Approval struct {
	SchemaVersion string     `json:"schema_version"`
	Context       ContextRef `json:"context"`
	Phase         Phase      `json:"phase"`
	Revision      uint64     `json:"revision"`
	ContractHash  string     `json:"contract_hash"`
	Actor         string     `json:"actor"`
	ApprovedAt    time.Time  `json:"approved_at"`
	Comment       string     `json:"comment,omitempty"`
}

func (a Approval) Validate() error {
	var errs ValidationErrors
	validateVersion(a.SchemaVersion, &errs)
	a.Context.validate("context", &errs)
	if !a.Phase.valid() {
		errs.add("phase", "is not a recognized phase")
	}
	if a.Revision == 0 {
		errs.add("revision", "must be greater than zero")
	}
	if !hashPattern.MatchString(a.ContractHash) {
		errs.add("contract_hash", "must use sha256:<64 lowercase hex characters>")
	}
	validateRequired("actor", a.Actor, &errs)
	if a.ApprovedAt.IsZero() {
		errs.add("approved_at", "must be set")
	}
	return validationResult(errs)
}

func (a Approval) ValidateContract(phase Phase, revision uint64, hash string) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.Phase != phase {
		return fmt.Errorf("approval phase %q does not match %q", a.Phase, phase)
	}
	if a.Revision != revision {
		return fmt.Errorf("approval revision %d does not match %d", a.Revision, revision)
	}
	if a.ContractHash != hash {
		return fmt.Errorf("approval hash does not match current contract")
	}
	return nil
}
