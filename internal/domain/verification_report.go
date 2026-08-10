package domain

import (
	"fmt"
	"time"
)

type VerificationVerdict string

const (
	VerificationPass         VerificationVerdict = "pass"
	VerificationFail         VerificationVerdict = "fail"
	VerificationBlocked      VerificationVerdict = "blocked"
	VerificationInconclusive VerificationVerdict = "inconclusive"
	VerificationPending      VerificationVerdict = "pending"
)

type JudgmentStatus string

const (
	JudgmentPass    JudgmentStatus = "pass"
	JudgmentFail    JudgmentStatus = "fail"
	JudgmentBlocked JudgmentStatus = "blocked"
	JudgmentPending JudgmentStatus = "pending"
)

type VerificationEvidence struct {
	Command          []string `json:"command"`
	WorkingDirectory string   `json:"working_directory"`
	ExitCode         int      `json:"exit_code"`
	Stdout           string   `json:"stdout,omitempty"`
	Stderr           string   `json:"stderr,omitempty"`
	StdoutHash       string   `json:"stdout_hash"`
	StderrHash       string   `json:"stderr_hash"`
	DurationMillis   int64    `json:"duration_millis"`
	Seed             *uint64  `json:"seed,omitempty"`
	Iterations       *uint64  `json:"iterations,omitempty"`
	Judge            string   `json:"judge,omitempty"`
}

type JudgmentOutcome struct {
	JudgmentID             string               `json:"judgment_id"`
	Status                 JudgmentStatus       `json:"status"`
	RequirementIDs         []string             `json:"requirement_ids"`
	AcceptanceCriterionIDs []string             `json:"acceptance_criterion_ids"`
	SurfaceIDs             []string             `json:"surface_ids"`
	Summary                string               `json:"summary"`
	Evidence               VerificationEvidence `json:"evidence"`
}

type SurfaceResolution struct {
	SurfaceID   string `json:"surface_id"`
	ModelNodeID string `json:"model_node_id,omitempty"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
}

type CoverageOutcome struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Rationale string `json:"rationale,omitempty"`
}

type VerificationReport struct {
	SchemaVersion      string              `json:"schema_version"`
	Context            ContextRef          `json:"context"`
	Producer           Producer            `json:"producer"`
	Attempt            uint64              `json:"attempt"`
	CandidateCommit    string              `json:"candidate_commit"`
	DesignPackageHash  string              `json:"design_package_hash"`
	CandidateModel     ModelReference      `json:"candidate_model"`
	Outcomes           []JudgmentOutcome   `json:"outcomes"`
	Requirements       []CoverageOutcome   `json:"requirements"`
	AcceptanceCriteria []CoverageOutcome   `json:"acceptance_criteria"`
	SurfaceResolutions []SurfaceResolution `json:"surface_resolutions"`
	Findings           []string            `json:"findings,omitempty"`
	Verdict            VerificationVerdict `json:"verdict"`
	StartedAt          time.Time           `json:"started_at"`
	CompletedAt        time.Time           `json:"completed_at"`
}

type VerificationAttempt struct {
	Number           uint64             `json:"number"`
	CandidateCommit  string             `json:"candidate_commit"`
	PackageHash      string             `json:"package_hash"`
	ArtifactRootHash string             `json:"artifact_root_hash"`
	BaseModel        ModelReference     `json:"base_model"`
	CandidateModel   ModelReference     `json:"candidate_model"`
	Workspace        Workspace          `json:"workspace"`
	Report           VerificationReport `json:"report"`
	ReportHash       string             `json:"report_hash"`
}

type RemediationNeed struct {
	RequirementIDs         []string `json:"requirement_ids"`
	AcceptanceCriterionIDs []string `json:"acceptance_criterion_ids"`
	SurfaceIDs             []string `json:"surface_ids"`
	Expected               string   `json:"expected"`
	Actual                 string   `json:"actual"`
}

type RemediationSpec struct {
	SchemaVersion    string            `json:"schema_version"`
	Context          ContextRef        `json:"context"`
	FailedReportHash string            `json:"failed_report_hash"`
	Needs            []RemediationNeed `json:"needs"`
	CreatedAt        time.Time         `json:"created_at"`
}

type GherkinPublication struct {
	Attempt           uint64         `json:"attempt"`
	VerifiedCommit    string         `json:"verified_commit"`
	PublicationCommit string         `json:"publication_commit"`
	Model             ModelReference `json:"model"`
	PublishedPaths    []string       `json:"published_paths"`
	PublishedAt       time.Time      `json:"published_at"`
}

func (r VerificationReport) Validate() error {
	var errs ValidationErrors
	validateVersion(r.SchemaVersion, &errs)
	r.Context.validate("context", &errs)
	r.Producer.validate("producer", &errs)
	if r.Attempt == 0 || r.Attempt > 2 {
		errs.add("attempt", "must be one or two")
	}
	if !gitCommitPattern.MatchString(r.CandidateCommit) {
		errs.add("candidate_commit", "must be a full Git commit ID")
	}
	if !hashPattern.MatchString(r.DesignPackageHash) {
		errs.add("design_package_hash", "must use sha256:<64 lowercase hex characters>")
	}
	r.CandidateModel.validate("candidate_model", &errs)
	if r.Verdict != VerificationBlocked && r.CandidateModel.SourceRevision != r.CandidateCommit {
		errs.add("candidate_model.source_revision", "must match candidate_commit for completed verification")
	}
	if len(r.Outcomes) == 0 {
		errs.add("outcomes", "must contain every required judgment")
	}
	seen := map[string]struct{}{}
	hasFailure, hasBlocked := false, false
	for i, outcome := range r.Outcomes {
		path := fmt.Sprintf("outcomes[%d]", i)
		validateID(path+".judgment_id", outcome.JudgmentID, &errs)
		if _, ok := seen[outcome.JudgmentID]; ok {
			errs.add(path+".judgment_id", "must be unique")
		}
		seen[outcome.JudgmentID] = struct{}{}
		if outcome.Status != JudgmentPass && outcome.Status != JudgmentFail && outcome.Status != JudgmentBlocked {
			errs.add(path+".status", "must be pass, fail, or blocked")
		}
		hasFailure = hasFailure || outcome.Status == JudgmentFail
		hasBlocked = hasBlocked || outcome.Status == JudgmentBlocked
		validateRequiredIDs(path+".requirement_ids", outcome.RequirementIDs, true, &errs)
		validateRequiredIDs(path+".acceptance_criterion_ids", outcome.AcceptanceCriterionIDs, true, &errs)
		validateRequiredIDs(path+".surface_ids", outcome.SurfaceIDs, true, &errs)
		validateRequired(path+".summary", outcome.Summary, &errs)
		if !hashPattern.MatchString(outcome.Evidence.StdoutHash) {
			errs.add(path+".evidence.stdout_hash", "must use sha256:<64 lowercase hex characters>")
		}
		if !hashPattern.MatchString(outcome.Evidence.StderrHash) {
			errs.add(path+".evidence.stderr_hash", "must use sha256:<64 lowercase hex characters>")
		}
		if outcome.Status != JudgmentBlocked && len(outcome.Evidence.Command) == 0 {
			errs.add(path+".evidence.command", "must record the approved argv")
		}
	}
	if r.Verdict != VerificationPass && r.Verdict != VerificationFail &&
		r.Verdict != VerificationBlocked && r.Verdict != VerificationInconclusive {
		errs.add("verdict", "must be pass, fail, blocked, or inconclusive")
	}
	if r.Verdict == VerificationPass && (hasFailure || hasBlocked) {
		errs.add("verdict", "pass requires every judgment to pass")
	}
	if r.Verdict == VerificationFail && (!hasFailure || hasBlocked) {
		errs.add("verdict", "fail requires at least one failure and no blocked judgment")
	}
	if r.Verdict == VerificationBlocked && !hasBlocked && len(r.Findings) == 0 {
		errs.add("verdict", "blocked requires a blocked judgment or finding")
	}
	validateCoverageOutcomes("requirements", r.Requirements, &errs)
	validateCoverageOutcomes("acceptance_criteria", r.AcceptanceCriteria, &errs)
	if r.StartedAt.IsZero() || r.CompletedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) {
		errs.add("completed_at", "must be at or after a set started_at")
	}
	return validationResult(errs)
}

func validateCoverageOutcomes(path string, outcomes []CoverageOutcome, errs *ValidationErrors) {
	if len(outcomes) == 0 {
		errs.add(path, "must aggregate covered traceability targets")
	}
	seen := map[string]struct{}{}
	for i, outcome := range outcomes {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		validateID(itemPath+".id", outcome.ID, errs)
		if _, ok := seen[outcome.ID]; ok {
			errs.add(itemPath+".id", "must be unique")
		}
		seen[outcome.ID] = struct{}{}
		if outcome.Status != string(JudgmentPass) && outcome.Status != string(JudgmentFail) &&
			outcome.Status != string(JudgmentBlocked) && outcome.Status != "excluded" {
			errs.add(itemPath+".status", "must be pass, fail, blocked, or excluded")
		}
		if outcome.Status == "excluded" && outcome.Rationale == "" {
			errs.add(itemPath+".rationale", "must explain an approved exclusion")
		}
	}
}
