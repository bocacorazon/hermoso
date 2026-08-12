package domain

import (
	"fmt"
	"time"

	"github.com/bocacorazon/hermoso/internal/digest"
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
	StatusAwaitingJudgment     RunStatus = "awaiting_judgment"
	StatusAwaitingRemediation  RunStatus = "awaiting_remediation"
)

type Run struct {
	SchemaVersion         string                `json:"schema_version"`
	Context               ContextRef            `json:"context"`
	Phase                 Phase                 `json:"phase"`
	Status                RunStatus             `json:"status"`
	Revision              uint64                `json:"revision"`
	CreatedAt             time.Time             `json:"created_at"`
	UpdatedAt             time.Time             `json:"updated_at"`
	Design                *DesignState          `json:"design,omitempty"`
	ConstructionRounds    []ConstructionState   `json:"construction_rounds,omitempty"`
	VerificationAttempts  []VerificationAttempt `json:"verification_attempts,omitempty"`
	VerificationIncidents []VerificationAttempt `json:"verification_incidents,omitempty"`
	VerificationBlocker   string                `json:"verification_blocker,omitempty"`
	Publication           *GherkinPublication   `json:"gherkin_publication,omitempty"`
	TaskBindings          []TaskBinding         `json:"task_bindings,omitempty"`
	Evidence              []Evidence            `json:"evidence,omitempty"`
	ProfilePath           string                `json:"profile_path,omitempty"`
}

type DesignState struct {
	Revision         uint64                       `json:"revision"`
	Feature          FeatureDesign                `json:"feature_design"`
	FeatureHash      string                       `json:"feature_hash"`
	Verification     *FeatureVerificationContract `json:"verification_contract,omitempty"`
	VerificationHash string                       `json:"verification_hash,omitempty"`
	ArtifactRootHash string                       `json:"artifact_root_hash,omitempty"`
	PackageHash      string                       `json:"package_hash,omitempty"`
	Approval         *Approval                    `json:"approval,omitempty"`
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
	Number                  uint64             `json:"number"`
	Kind                    string             `json:"kind"`
	SourceHash              string             `json:"source_hash"`
	Remediation             *RemediationSpec   `json:"remediation_spec,omitempty"`
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

func (r *Run) CurrentConstruction() *ConstructionState {
	if r == nil || len(r.ConstructionRounds) == 0 {
		return nil
	}
	return &r.ConstructionRounds[len(r.ConstructionRounds)-1]
}

func (r Run) LatestConstruction() *ConstructionState {
	if len(r.ConstructionRounds) == 0 {
		return nil
	}
	round := r.ConstructionRounds[len(r.ConstructionRounds)-1]
	return &round
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
		if r.Design.Revision == 0 {
			errs.add("design.revision", "must be greater than zero")
		}
		if err := r.Design.Feature.Validate(); err != nil {
			appendNested("design.feature_design", err, &errs)
		}
		if !r.Design.Feature.Context.Equal(r.Context) {
			errs.add("design.feature_design.context", "must match run context")
		}
		if !hashPattern.MatchString(r.Design.FeatureHash) {
			errs.add("design.feature_hash", "must use sha256:<64 lowercase hex characters>")
		}
		if r.Design.Verification == nil {
			if r.Design.VerificationHash != "" || r.Design.ArtifactRootHash != "" ||
				r.Design.PackageHash != "" || r.Design.Approval != nil {
				errs.add("design", "incomplete design package must not contain verification hashes or approval")
			}
		} else {
			if err := r.Design.Verification.Validate(); err != nil {
				appendNested("design.verification_contract", err, &errs)
			}
			if !r.Design.Verification.Context.Equal(r.Context) {
				errs.add("design.verification_contract.context", "must match run context")
			}
			for path, value := range map[string]string{
				"design.verification_hash":  r.Design.VerificationHash,
				"design.artifact_root_hash": r.Design.ArtifactRootHash,
				"design.package_hash":       r.Design.PackageHash,
			} {
				if !hashPattern.MatchString(value) {
					errs.add(path, "must use sha256:<64 lowercase hex characters>")
				}
			}
			expected, err := DesignPackageHash(
				r.Design.Revision, r.Design.FeatureHash, r.Design.VerificationHash,
				r.Design.ArtifactRootHash, r.Design.Feature.BaseModel,
			)
			if err != nil {
				errs.add("design.package_hash", err.Error())
			} else if r.Design.PackageHash != expected {
				errs.add("design.package_hash", "does not match design package content")
			}
			if r.Design.Approval != nil {
				if err := r.Design.Approval.ValidatePackage(PhaseDesign, r.Design.Revision, r.Design.PackageHash); err != nil {
					errs.add("design.approval", err.Error())
				}
				if !r.Design.Approval.Context.Equal(r.Context) {
					errs.add("design.approval.context", "must match run context")
				}
			}
		}
	}
	for roundIndex := range r.ConstructionRounds {
		construction := &r.ConstructionRounds[roundIndex]
		prefix := fmt.Sprintf("construction_rounds[%d]", roundIndex)
		if construction.Number != uint64(roundIndex+1) {
			errs.add(prefix+".number", "must be sequential starting at one")
		}
		if (roundIndex == 0 && construction.Kind != "initial") ||
			(roundIndex > 0 && construction.Kind != "remediation") {
			errs.add(prefix+".kind", "must be initial for round one and remediation thereafter")
		}
		if construction.Kind == "initial" && construction.Remediation != nil {
			errs.add(prefix+".remediation_spec", "must be empty for initial construction")
		}
		if construction.Kind == "remediation" {
			if construction.Remediation == nil {
				errs.add(prefix+".remediation_spec", "must be set for remediation construction")
			} else {
				if !construction.Remediation.Context.Equal(r.Context) {
					errs.add(prefix+".remediation_spec.context", "must match run context")
				}
				if construction.Remediation.FailedReportHash != construction.SourceHash {
					errs.add(prefix+".remediation_spec.failed_report_hash", "must match source_hash")
				}
				if len(construction.Remediation.Needs) == 0 {
					errs.add(prefix+".remediation_spec.needs", "must not be empty")
				}
			}
		}
		if !hashPattern.MatchString(construction.SourceHash) {
			errs.add(prefix+".source_hash", "must use sha256:<64 lowercase hex characters>")
		}
		if roundIndex == 0 && r.Design != nil && construction.SourceHash != r.Design.PackageHash {
			errs.add(prefix+".source_hash", "must match approved design package")
		}
		if roundIndex > 0 && roundIndex-1 < len(r.VerificationAttempts) &&
			construction.SourceHash != r.VerificationAttempts[roundIndex-1].ReportHash {
			errs.add(prefix+".source_hash", "must match the preceding failed verification report")
		}
		if err := construction.Graph.Validate(); err != nil {
			appendNested(prefix+".graph", err, &errs)
		}
		if !construction.Graph.Context.Equal(r.Context) {
			errs.add(prefix+".graph.context", "must match run context")
		}
		if !hashPattern.MatchString(construction.Hash) {
			errs.add(prefix+".hash", "must use sha256:<64 lowercase hex characters>")
		}
		seenItems := map[string]struct{}{}
		for i, item := range construction.Items {
			path := fmt.Sprintf("%s.items[%d]", prefix, i)
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
		if construction.IntegratedAt == nil {
			if construction.IntegratedFeatureCommit != "" || len(construction.IntegratedLeaves) != 0 {
				errs.add(prefix+".integrated_at", "must be set when integrated commit identities are recorded")
			}
		} else {
			if !gitCommitPattern.MatchString(construction.IntegratedFeatureCommit) {
				errs.add(prefix+".integrated_feature_commit", "must be a full Git commit ID")
			}
			if len(construction.IntegratedLeaves) == 0 {
				errs.add(prefix+".integrated_leaves", "must contain at least one leaf commit")
			}
			seenLeaves := map[string]struct{}{}
			for i, leaf := range construction.IntegratedLeaves {
				path := fmt.Sprintf("%s.integrated_leaves[%d]", prefix, i)
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
		if construction.Result != nil {
			if err := construction.Result.Validate(); err != nil {
				appendNested(prefix+".result", err, &errs)
			}
			if !construction.Result.Context.Equal(r.Context) {
				errs.add(prefix+".result.context", "must match run context")
			}
		}
	}
	for attemptIndex := range r.VerificationAttempts {
		attempt := &r.VerificationAttempts[attemptIndex]
		prefix := fmt.Sprintf("verification_attempts[%d]", attemptIndex)
		if attempt.Number != uint64(attemptIndex+1) || attempt.Number > 2 {
			errs.add(prefix+".number", "must be sequential and no greater than two")
		}
		if !gitCommitPattern.MatchString(attempt.CandidateCommit) {
			errs.add(prefix+".candidate_commit", "must be a full Git commit ID")
		}
		for path, hash := range map[string]string{
			prefix + ".package_hash":       attempt.PackageHash,
			prefix + ".artifact_root_hash": attempt.ArtifactRootHash,
			prefix + ".report_hash":        attempt.ReportHash,
		} {
			if !hashPattern.MatchString(hash) {
				errs.add(path, "must use sha256:<64 lowercase hex characters>")
			}
		}
		attempt.BaseModel.validate(prefix+".base_model", &errs)
		attempt.CandidateModel.validate(prefix+".candidate_model", &errs)
		if err := attempt.Report.Validate(); err != nil {
			appendNested(prefix+".report", err, &errs)
		}
		if reportHash, err := digest.JSON(attempt.Report); err != nil {
			errs.add(prefix+".report_hash", err.Error())
		} else if reportHash != attempt.ReportHash {
			errs.add(prefix+".report_hash", "does not match report content")
		}
		if attempt.Report.Attempt != attempt.Number ||
			attempt.Report.CandidateCommit != attempt.CandidateCommit ||
			attempt.Report.DesignPackageHash != attempt.PackageHash {
			errs.add(prefix+".report", "must match attempt identity")
		}
		if attempt.Report.CandidateModel != attempt.CandidateModel {
			errs.add(prefix+".candidate_model", "must match report candidate_model")
		}
		if attemptIndex < len(r.ConstructionRounds) {
			round := r.ConstructionRounds[attemptIndex]
			if attempt.CandidateCommit != round.IntegratedFeatureCommit {
				errs.add(prefix+".candidate_commit", "must match the corresponding integrated construction round")
			}
		}
		if r.Design != nil && (attempt.PackageHash != r.Design.PackageHash ||
			attempt.ArtifactRootHash != r.Design.ArtifactRootHash) {
			errs.add(prefix, "must bind the current immutable design package")
		}
	}
	for incidentIndex := range r.VerificationIncidents {
		incident := &r.VerificationIncidents[incidentIndex]
		prefix := fmt.Sprintf("verification_incidents[%d]", incidentIndex)
		if incident.Number == 0 || incident.Number > 2 {
			errs.add(prefix+".number", "must reference verification attempt one or two")
		}
		if incident.Report.Verdict != VerificationBlocked &&
			incident.Report.Verdict != VerificationInconclusive {
			errs.add(prefix+".report.verdict", "must be blocked or inconclusive")
		}
		if !gitCommitPattern.MatchString(incident.CandidateCommit) {
			errs.add(prefix+".candidate_commit", "must be a full Git commit ID")
		}
		for path, hash := range map[string]string{
			prefix + ".package_hash":       incident.PackageHash,
			prefix + ".artifact_root_hash": incident.ArtifactRootHash,
			prefix + ".report_hash":        incident.ReportHash,
		} {
			if !hashPattern.MatchString(hash) {
				errs.add(path, "must use sha256:<64 lowercase hex characters>")
			}
		}
		incident.BaseModel.validate(prefix+".base_model", &errs)
		incident.CandidateModel.validate(prefix+".candidate_model", &errs)
		if err := incident.Report.Validate(); err != nil {
			appendNested(prefix+".report", err, &errs)
		}
		if reportHash, err := digest.JSON(incident.Report); err != nil {
			errs.add(prefix+".report_hash", err.Error())
		} else if reportHash != incident.ReportHash {
			errs.add(prefix+".report_hash", "does not match report content")
		}
		if incident.Report.Attempt != incident.Number ||
			incident.Report.CandidateCommit != incident.CandidateCommit ||
			incident.Report.DesignPackageHash != incident.PackageHash {
			errs.add(prefix+".report", "must match incident identity")
		}
		if incident.Report.CandidateModel != incident.CandidateModel {
			errs.add(prefix+".candidate_model", "must match report candidate_model")
		}
		if incident.Number <= uint64(len(r.ConstructionRounds)) &&
			incident.CandidateCommit != r.ConstructionRounds[incident.Number-1].IntegratedFeatureCommit {
			errs.add(prefix+".candidate_commit", "must match the corresponding integrated construction round")
		}
		if r.Design != nil && (incident.PackageHash != r.Design.PackageHash ||
			incident.ArtifactRootHash != r.Design.ArtifactRootHash) {
			errs.add(prefix, "must bind the current immutable design package")
		}
	}
	if r.Publication != nil {
		if r.Publication.Attempt == 0 || r.Publication.Attempt > uint64(len(r.VerificationAttempts)) {
			errs.add("gherkin_publication.attempt", "must reference a persisted verification attempt")
		}
		if !gitCommitPattern.MatchString(r.Publication.VerifiedCommit) ||
			!gitCommitPattern.MatchString(r.Publication.PublicationCommit) {
			errs.add("gherkin_publication", "must contain full verified and publication Git commits")
		}
		r.Publication.Model.validate("gherkin_publication.model", &errs)
		validateStringList("gherkin_publication.published_paths", r.Publication.PublishedPaths, true, &errs)
		if r.Publication.PublishedAt.IsZero() {
			errs.add("gherkin_publication.published_at", "must be set")
		}
		if r.Publication.Model.SourceRevision != r.Publication.PublicationCommit {
			errs.add("gherkin_publication.model.source_revision", "must match publication_commit")
		}
		if r.Publication.Attempt > 0 && r.Publication.Attempt <= uint64(len(r.VerificationAttempts)) {
			attempt := r.VerificationAttempts[r.Publication.Attempt-1]
			if attempt.Report.Verdict != VerificationPass ||
				r.Publication.VerifiedCommit != attempt.CandidateCommit {
				errs.add("gherkin_publication", "must reference a passing verification attempt")
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
		if binding.Round > uint64(len(r.ConstructionRounds)) {
			errs.add(path+".round", "references an unknown construction round")
		} else if binding.Round > 0 {
			found := false
			for _, item := range r.ConstructionRounds[binding.Round-1].Graph.Items {
				found = found || item.ID == binding.WorkItemID
			}
			if !found {
				errs.add(path+".work_item_id", "references an unknown work item in its construction round")
			}
		}
		bindingKey := fmt.Sprintf("%d:%s", binding.Round, binding.WorkItemID)
		if _, ok := bindings[bindingKey]; ok {
			errs.add(path+".work_item_id", "must not have more than one task binding")
		}
		bindings[bindingKey] = struct{}{}
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
		return status == StatusPending || status == StatusInProgress || status == StatusCompleted || status == StatusAwaitingVerification || status == StatusAwaitingRemediation
	case PhaseVerification:
		return status == StatusPending || status == StatusInProgress || status == StatusCompleted || status == StatusAwaitingRelease || status == StatusAwaitingJudgment
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
	{PhaseConstruction, StatusAwaitingVerification}: {
		{PhaseVerification, StatusPending}: {}, {PhaseVerification, StatusInProgress}: {},
		{PhaseVerification, StatusAwaitingJudgment}: {},
		{PhaseConstruction, StatusPending}: {}, {PhaseConstruction, StatusAwaitingRemediation}: {},
		{PhaseVerification, StatusBlocked}: {},
		{PhaseConstruction, StatusCancelled}: {},
	},
	{PhaseVerification, StatusPending}: {
		{PhaseVerification, StatusInProgress}: {}, {PhaseVerification, StatusCancelled}: {},
	},
	{PhaseVerification, StatusInProgress}: {
		{PhaseVerification, StatusBlocked}: {}, {PhaseVerification, StatusAwaitingRelease}: {}, {PhaseVerification, StatusCancelled}: {},
		{PhaseVerification, StatusAwaitingJudgment}: {},
	},
	{PhaseVerification, StatusAwaitingJudgment}: {
		{PhaseVerification, StatusInProgress}: {}, {PhaseVerification, StatusBlocked}: {},
		{PhaseVerification, StatusAwaitingRelease}: {}, {PhaseVerification, StatusCancelled}: {},
		{PhaseConstruction, StatusPending}: {},
		{PhaseConstruction, StatusAwaitingRemediation}: {},
	},
	{PhaseConstruction, StatusAwaitingRemediation}: {
		{PhaseConstruction, StatusPending}: {}, {PhaseConstruction, StatusBlocked}: {},
		{PhaseConstruction, StatusCancelled}: {},
	},
	{PhaseVerification, StatusBlocked}: {
		{PhaseVerification, StatusInProgress}: {}, {PhaseConstruction, StatusAwaitingVerification}: {},
		{PhaseVerification, StatusAwaitingRelease}: {}, {PhaseVerification, StatusCancelled}: {},
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
