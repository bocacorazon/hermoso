package domain

import (
	"fmt"
	"time"
)

type Phase string

const (
	PhaseDesign       Phase = "design"
	PhaseConstruction Phase = "construction"
	PhaseVerification Phase = "verification"
	PhaseRelease      Phase = "release"
)

type RunStatus string

const (
	StatusPending              RunStatus = "pending"
	StatusInProgress           RunStatus = "in_progress"
	StatusAwaitingApproval     RunStatus = "awaiting_approval"
	StatusBlocked              RunStatus = "blocked"
	StatusCompleted            RunStatus = "completed"
	StatusAwaitingVerification RunStatus = "awaiting_verification"
	StatusAwaitingRelease      RunStatus = "awaiting_release"
	StatusReleased             RunStatus = "released"
	StatusCancelled            RunStatus = "cancelled"
)

type Run struct {
	SchemaVersion string             `json:"schema_version"`
	Context       ContextRef         `json:"context"`
	Phase         Phase              `json:"phase"`
	Status        RunStatus          `json:"status"`
	Revision      uint64             `json:"revision"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	Design        *DesignState       `json:"design,omitempty"`
	Construction  *ConstructionState `json:"construction,omitempty"`
	TaskBindings  []TaskBinding      `json:"task_bindings,omitempty"`
	Evidence      []Evidence         `json:"evidence,omitempty"`
}

type DesignState struct {
	Contract FeatureDesign `json:"contract"`
	Hash     string        `json:"hash"`
	Approval *Approval     `json:"approval,omitempty"`
}

type Workspace struct {
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	Branch       string `json:"branch"`
	Path         string `json:"path"`
	ParentBranch string `json:"parent_branch"`
	ParentCommit string `json:"parent_commit"`
}

type WorkStatus string

const (
	WorkPending   WorkStatus = "pending"
	WorkStarted   WorkStatus = "started"
	WorkCompleted WorkStatus = "completed"
	WorkBlocked   WorkStatus = "blocked"
)

type WorkState struct {
	ID        string     `json:"id"`
	Status    WorkStatus `json:"status"`
	Workspace Workspace  `json:"workspace"`
	Blocker   string     `json:"blocker,omitempty"`
	Evidence  []Evidence `json:"evidence,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type ConstructionState struct {
	Graph                   WorkGraph          `json:"graph"`
	Hash                    string             `json:"hash"`
	ProfilePath             string             `json:"profile_path,omitempty"`
	Feature                 Workspace          `json:"feature_workspace,omitempty"`
	Items                   []WorkState        `json:"items,omitempty"`
	IntegratedAt            *time.Time         `json:"integrated_at,omitempty"`
	IntegratedFeatureCommit string             `json:"integrated_feature_commit,omitempty"`
	IntegratedLeaves        []IntegratedCommit `json:"integrated_leaves,omitempty"`
	Result                  *PhaseResult       `json:"result,omitempty"`
	Blocker                 string             `json:"blocker,omitempty"`
}

type IntegratedCommit struct {
	ID     string `json:"id"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
}

func (r Run) Validate() error {
	var errs ValidationErrors
	validateVersion(r.SchemaVersion, &errs)
	r.Context.validate("context", &errs)
	if !r.Phase.valid() {
		errs.add("phase", "is not a recognized phase")
	}
	if !statusAllowed(r.Phase, r.Status) {
		errs.add("status", fmt.Sprintf("%q is not legal for phase %q", r.Status, r.Phase))
	}
	if r.Revision == 0 {
		errs.add("revision", "must be greater than zero")
	}
	if r.CreatedAt.IsZero() {
		errs.add("created_at", "must be set")
	}
	if r.UpdatedAt.IsZero() {
		errs.add("updated_at", "must be set")
	} else if !r.CreatedAt.IsZero() && r.UpdatedAt.Before(r.CreatedAt) {
		errs.add("updated_at", "must not precede created_at")
	}
	if r.Design != nil {
		if err := r.Design.Contract.Validate(); err != nil {
			appendNested("design.contract", err, &errs)
		}
		if !r.Design.Contract.Context.Equal(r.Context) {
			errs.add("design.contract.context", "must match run context")
		}
		if !hashPattern.MatchString(r.Design.Hash) {
			errs.add("design.hash", "must use sha256:<64 lowercase hex characters>")
		}
		if r.Design.Approval != nil {
			if err := r.Design.Approval.ValidateContract(PhaseDesign, r.Design.Contract.Revision, r.Design.Hash); err != nil {
				errs.add("design.approval", err.Error())
			}
			if !r.Design.Approval.Context.Equal(r.Context) {
				errs.add("design.approval.context", "must match run context")
			}
		}
	}
	if r.Construction != nil {
		if err := r.Construction.Graph.Validate(); err != nil {
			appendNested("construction.graph", err, &errs)
		}
		if !r.Construction.Graph.Context.Equal(r.Context) {
			errs.add("construction.graph.context", "must match run context")
		}
		if !hashPattern.MatchString(r.Construction.Hash) {
			errs.add("construction.hash", "must use sha256:<64 lowercase hex characters>")
		}
		seenItems := map[string]struct{}{}
		for i, item := range r.Construction.Items {
			path := fmt.Sprintf("construction.items[%d]", i)
			validateID(path+".id", item.ID, &errs)
			if _, ok := seenItems[item.ID]; ok {
				errs.add(path+".id", "must be unique")
			}
			seenItems[item.ID] = struct{}{}
			if item.Status != WorkPending && item.Status != WorkStarted && item.Status != WorkCompleted && item.Status != WorkBlocked {
				errs.add(path+".status", "must be pending, started, completed, or blocked")
			}
			for j, evidence := range item.Evidence {
				evidence.validate(fmt.Sprintf("%s.evidence[%d]", path, j), &errs)
				if !evidence.Context.Equal(r.Context) {
					errs.add(fmt.Sprintf("%s.evidence[%d].context", path, j), "must match run context")
				}
			}
		}
		if r.Construction.IntegratedAt == nil {
			if r.Construction.IntegratedFeatureCommit != "" || len(r.Construction.IntegratedLeaves) != 0 {
				errs.add("construction.integrated_at", "must be set when integrated commit identities are recorded")
			}
		} else {
			if !gitCommitPattern.MatchString(r.Construction.IntegratedFeatureCommit) {
				errs.add("construction.integrated_feature_commit", "must be a full Git commit ID")
			}
			if len(r.Construction.IntegratedLeaves) == 0 {
				errs.add("construction.integrated_leaves", "must contain at least one leaf commit")
			}
			seenLeaves := map[string]struct{}{}
			for i, leaf := range r.Construction.IntegratedLeaves {
				path := fmt.Sprintf("construction.integrated_leaves[%d]", i)
				validateID(path+".id", leaf.ID, &errs)
				validateRequired(path+".branch", leaf.Branch, &errs)
				if !gitCommitPattern.MatchString(leaf.Commit) {
					errs.add(path+".commit", "must be a full Git commit ID")
				}
				if _, ok := seenLeaves[leaf.ID]; ok {
					errs.add(path+".id", "must be unique")
				}
				seenLeaves[leaf.ID] = struct{}{}
			}
		}
		if r.Construction.Result != nil {
			if err := r.Construction.Result.Validate(); err != nil {
				appendNested("construction.result", err, &errs)
			}
			if !r.Construction.Result.Context.Equal(r.Context) {
				errs.add("construction.result.context", "must match run context")
			}
		}
	}
	bindings := make(map[string]struct{}, len(r.TaskBindings))
	boundTasks := make(map[string]struct{}, len(r.TaskBindings))
	for i, binding := range r.TaskBindings {
		path := fmt.Sprintf("task_bindings[%d]", i)
		if err := binding.Validate(); err != nil {
			appendNested(path, err, &errs)
		}
		if !binding.Context.Equal(r.Context) {
			errs.add(path+".context", "must match run context")
		}
		if _, ok := bindings[binding.WorkItemID]; ok {
			errs.add(path+".work_item_id", "must not have more than one task binding")
		}
		bindings[binding.WorkItemID] = struct{}{}
		if _, ok := boundTasks[binding.TaskID]; ok {
			errs.add(path+".task_id", "must not be bound to more than one work item")
		}
		boundTasks[binding.TaskID] = struct{}{}
	}
	evidence := make(map[string]struct{}, len(r.Evidence))
	for i, item := range r.Evidence {
		path := fmt.Sprintf("evidence[%d]", i)
		item.validate(path, &errs)
		if !item.Context.Equal(r.Context) {
			errs.add(path+".context", "must match run context")
		}
		if _, ok := evidence[item.ID]; ok {
			errs.add(path+".id", "must be unique")
		}
		evidence[item.ID] = struct{}{}
	}
	return validationResult(errs)
}

func (p Phase) valid() bool {
	return p == PhaseDesign || p == PhaseConstruction || p == PhaseVerification || p == PhaseRelease
}

func statusAllowed(phase Phase, status RunStatus) bool {
	if status == StatusBlocked || status == StatusCancelled {
		return phase.valid()
	}
	switch phase {
	case PhaseDesign:
		return status == StatusPending || status == StatusInProgress || status == StatusAwaitingApproval || status == StatusCompleted
	case PhaseConstruction:
		return status == StatusPending || status == StatusInProgress || status == StatusCompleted || status == StatusAwaitingVerification
	case PhaseVerification:
		return status == StatusPending || status == StatusInProgress || status == StatusCompleted || status == StatusAwaitingRelease
	case PhaseRelease:
		return status == StatusPending || status == StatusInProgress || status == StatusCompleted || status == StatusReleased
	default:
		return false
	}
}

type lifecycleState struct {
	phase  Phase
	status RunStatus
}

var legalTransitions = map[lifecycleState]map[lifecycleState]struct{}{
	{PhaseDesign, StatusPending}: {
		{PhaseDesign, StatusInProgress}: {}, {PhaseDesign, StatusAwaitingApproval}: {}, {PhaseDesign, StatusCancelled}: {},
	},
	{PhaseDesign, StatusInProgress}: {
		{PhaseDesign, StatusAwaitingApproval}: {}, {PhaseDesign, StatusBlocked}: {}, {PhaseDesign, StatusCancelled}: {},
	},
	{PhaseDesign, StatusBlocked}: {
		{PhaseDesign, StatusInProgress}: {}, {PhaseDesign, StatusCancelled}: {},
	},
	{PhaseDesign, StatusAwaitingApproval}: {
		{PhaseConstruction, StatusPending}: {}, {PhaseDesign, StatusInProgress}: {}, {PhaseDesign, StatusCancelled}: {},
	},
	{PhaseConstruction, StatusPending}: {
		{PhaseDesign, StatusAwaitingApproval}: {}, {PhaseConstruction, StatusInProgress}: {}, {PhaseConstruction, StatusCancelled}: {},
	},
	{PhaseConstruction, StatusInProgress}: {
		{PhaseConstruction, StatusBlocked}: {}, {PhaseConstruction, StatusAwaitingVerification}: {}, {PhaseConstruction, StatusCancelled}: {},
	},
	{PhaseConstruction, StatusBlocked}: {
		{PhaseConstruction, StatusInProgress}: {}, {PhaseConstruction, StatusCancelled}: {},
	},
	// Verification and release states are intentionally reserved for future commands.
	{PhaseConstruction, StatusAwaitingVerification}: {
		{PhaseVerification, StatusPending}: {}, {PhaseConstruction, StatusCancelled}: {},
	},
	{PhaseVerification, StatusPending}: {
		{PhaseVerification, StatusInProgress}: {}, {PhaseVerification, StatusCancelled}: {},
	},
	{PhaseVerification, StatusInProgress}: {
		{PhaseVerification, StatusBlocked}: {}, {PhaseVerification, StatusAwaitingRelease}: {}, {PhaseVerification, StatusCancelled}: {},
	},
	{PhaseVerification, StatusBlocked}: {
		{PhaseVerification, StatusInProgress}: {}, {PhaseVerification, StatusCancelled}: {},
	},
	{PhaseVerification, StatusAwaitingRelease}: {
		{PhaseRelease, StatusPending}: {}, {PhaseVerification, StatusCancelled}: {},
	},
	{PhaseRelease, StatusPending}: {
		{PhaseRelease, StatusInProgress}: {}, {PhaseRelease, StatusCancelled}: {},
	},
	{PhaseRelease, StatusInProgress}: {
		{PhaseRelease, StatusBlocked}: {}, {PhaseRelease, StatusReleased}: {}, {PhaseRelease, StatusCancelled}: {},
	},
	{PhaseRelease, StatusBlocked}: {
		{PhaseRelease, StatusInProgress}: {}, {PhaseRelease, StatusCancelled}: {},
	},
}

func ValidateTransition(fromPhase Phase, fromStatus RunStatus, toPhase Phase, toStatus RunStatus) error {
	if !statusAllowed(fromPhase, fromStatus) {
		return fmt.Errorf("invalid source lifecycle state %s/%s", fromPhase, fromStatus)
	}
	if !statusAllowed(toPhase, toStatus) {
		return fmt.Errorf("invalid destination lifecycle state %s/%s", toPhase, toStatus)
	}
	if fromPhase == toPhase && fromStatus == toStatus {
		return nil
	}
	if _, ok := legalTransitions[lifecycleState{fromPhase, fromStatus}][lifecycleState{toPhase, toStatus}]; !ok {
		return fmt.Errorf("illegal lifecycle transition from %s/%s to %s/%s", fromPhase, fromStatus, toPhase, toStatus)
	}
	return nil
}

func appendNested(prefix string, err error, errs *ValidationErrors) {
	if nested, ok := err.(ValidationErrors); ok {
		for _, item := range nested {
			errs.add(prefix+"."+item.Path, item.Message)
		}
		return
	}
	errs.add(prefix, err.Error())
}
