package domain

import (
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

type Producer struct {
	Skill   string `json:"skill"`
	Runtime string `json:"runtime"`
	Version string `json:"version,omitempty"`
}

func (p Producer) validate(path string, errs *ValidationErrors) {
	validateRequired(path+".skill", p.Skill, errs)
	validateRequired(path+".runtime", p.Runtime, errs)
}

type TargetIdentity struct {
	Repository    string `json:"repository"`
	RemoteURL     string `json:"remote_url,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

func (t TargetIdentity) Equal(other TargetIdentity) bool {
	return t.Repository == other.Repository &&
		t.RemoteURL == other.RemoteURL &&
		t.DefaultBranch == other.DefaultBranch
}

type ContextRef struct {
	SchemaVersion string         `json:"schema_version"`
	ProjectID     string         `json:"project_id"`
	FeatureID     string         `json:"feature_id"`
	RunID         string         `json:"run_id"`
	Repository    TargetIdentity `json:"repository"`
}

type ExecutionContext = ContextRef

func (c ContextRef) Validate() error {
	var errs ValidationErrors
	c.validate("context", &errs)
	return validationResult(errs)
}

func (c ContextRef) validate(path string, errs *ValidationErrors) {
	if c.SchemaVersion != ContextSchemaVersion {
		errs.add(path+".schema_version", "must be "+ContextSchemaVersion)
	}
	validateID(path+".project_id", c.ProjectID, errs)
	validateID(path+".feature_id", c.FeatureID, errs)
	validateID(path+".run_id", c.RunID, errs)
	c.Repository.validate(path+".repository", errs)
}

func (c ContextRef) Equal(other ContextRef) bool {
	return c.SchemaVersion == other.SchemaVersion &&
		c.ProjectID == other.ProjectID &&
		c.FeatureID == other.FeatureID &&
		c.RunID == other.RunID &&
		c.Repository.Equal(other.Repository)
}

func KanbanTenant(projectID string) string {
	return "hermoso-" + projectID
}

func (t TargetIdentity) validate(path string, errs *ValidationErrors) {
	validateRequired(path+".repository", t.Repository, errs)
	if t.Repository != "" && !filepath.IsAbs(t.Repository) {
		errs.add(path+".repository", "must be an absolute path")
	}
	if t.RemoteURL != "" {
		if strings.ContainsAny(t.RemoteURL, "\r\n") {
			errs.add(path+".remote_url", "must not contain line breaks")
		} else if !isGitRemote(t.RemoteURL) {
			errs.add(path+".remote_url", "must be an absolute URL or scp-style Git remote")
		}
	}
	if strings.TrimSpace(t.DefaultBranch) == "" && t.DefaultBranch != "" {
		errs.add(path+".default_branch", "must not be blank")
	}
}

func isGitRemote(value string) bool {
	parsed, err := url.Parse(value)
	if err == nil && parsed.IsAbs() && parsed.Host != "" {
		return true
	}
	at := strings.IndexByte(value, '@')
	colon := strings.IndexByte(value, ':')
	return at > 0 && colon > at+1 && colon < len(value)-1
}

type Project struct {
	SchemaVersion string         `json:"schema_version"`
	ProjectID     string         `json:"project_id"`
	Target        TargetIdentity `json:"target"`
	KanbanTenant  string         `json:"kanban_tenant"`
	CreatedAt     time.Time      `json:"created_at"`
	ProfilePath   string         `json:"profile_path,omitempty"`
	// ConstitutionPath is an optional per-project override for the
	// constitution file. When set, it is the first candidate in the
	// constitution resolution order (see app.resolveConstitutionPath).
	ConstitutionPath string `json:"constitution_path,omitempty"`
}

func (p Project) Validate() error {
	var errs ValidationErrors
	if p.SchemaVersion != SchemaVersion {
		errs.add("schema_version", "must be "+SchemaVersion)
	}
	validateID("project_id", p.ProjectID, &errs)
	p.Target.validate("target", &errs)
	if p.KanbanTenant != KanbanTenant(p.ProjectID) {
		errs.add("kanban_tenant", "must be derived from project_id")
	}
	if p.CreatedAt.IsZero() {
		errs.add("created_at", "must be set")
	}
	return validationResult(errs)
}

type Evidence struct {
	Context     ContextRef `json:"context"`
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Path        string     `json:"path,omitempty"`
	Command     string     `json:"command,omitempty"`
	Summary     string     `json:"summary"`
	ContentHash string     `json:"content_hash,omitempty"`
	RecordedAt  time.Time  `json:"recorded_at"`
}

func (e Evidence) validate(path string, errs *ValidationErrors) {
	e.Context.validate(path+".context", errs)
	validateID(path+".id", e.ID, errs)
	validateRequired(path+".kind", e.Kind, errs)
	validateRequired(path+".summary", e.Summary, errs)
	if e.Path == "" && e.Command == "" {
		errs.add(path, "must include path or command")
	}
	if e.ContentHash != "" && !hashPattern.MatchString(e.ContentHash) {
		errs.add(path+".content_hash", "must use sha256:<64 lowercase hex characters>")
	}
	if e.RecordedAt.IsZero() {
		errs.add(path+".recorded_at", "must be set")
	}
}

type TaskBinding struct {
	Context    ContextRef `json:"context"`
	Round      uint64     `json:"round"`
	WorkItemID string     `json:"work_item_id"`
	TaskID     string     `json:"task_id"`
	BoundAt    time.Time  `json:"bound_at"`
}

func (b TaskBinding) Validate() error {
	var errs ValidationErrors
	b.Context.validate("context", &errs)
	if b.Round == 0 {
		errs.add("round", "must be greater than zero")
	}
	validateID("work_item_id", b.WorkItemID, &errs)
	validateRequired("task_id", b.TaskID, &errs)
	if strings.TrimSpace(b.TaskID) != b.TaskID || strings.ContainsAny(b.TaskID, "\r\n") {
		errs.add("task_id", "must not contain surrounding whitespace or line breaks")
	}
	if b.BoundAt.IsZero() {
		errs.add("bound_at", "must be set")
	}
	return validationResult(errs)
}
