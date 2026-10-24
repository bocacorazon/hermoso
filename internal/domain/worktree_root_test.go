package domain

import (
	"encoding/json"
	"testing"
)

func TestProjectWorktreeRootRoundTrips(t *testing.T) {
	t.Parallel()

	original := Project{
		SchemaVersion: SchemaVersion,
		ProjectID:     "project-test",
		Target: TargetIdentity{
			Repository: "/repos/test",
		},
		KanbanTenant: KanbanTenant("project-test"),
		CreatedAt:    domainTestTime,
		WorktreeRoot: "/home/user/Projects/.hermoso-worktrees",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored Project
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.WorktreeRoot != original.WorktreeRoot {
		t.Errorf("WorktreeRoot = %q, want %q", restored.WorktreeRoot, original.WorktreeRoot)
	}
}

func TestProjectWorktreeRootOmitsWhenEmpty(t *testing.T) {
	t.Parallel()

	project := Project{
		SchemaVersion: SchemaVersion,
		ProjectID:     "project-test",
		Target: TargetIdentity{
			Repository: "/repos/test",
		},
		KanbanTenant: KanbanTenant("project-test"),
		CreatedAt:    domainTestTime,
	}

	data, err := json.Marshal(project)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if _, present := raw["worktree_root"]; present {
		t.Errorf("worktree_root should be omitted when empty, got: %s", data)
	}
}

func TestProjectValidateRejectsRelativeWorktreeRoot(t *testing.T) {
	t.Parallel()

	project := Project{
		SchemaVersion: SchemaVersion,
		ProjectID:     "project-test",
		Target: TargetIdentity{
			Repository: "/repos/test",
		},
		KanbanTenant: KanbanTenant("project-test"),
		CreatedAt:    domainTestTime,
		WorktreeRoot: "relative/worktrees",
	}

	if err := project.Validate(); err == nil {
		t.Error("Validate should reject a relative worktree_root")
	}
}

func TestProjectValidateAcceptsAbsoluteWorktreeRoot(t *testing.T) {
	t.Parallel()

	project := Project{
		SchemaVersion: SchemaVersion,
		ProjectID:     "project-test",
		Target: TargetIdentity{
			Repository: "/repos/test",
		},
		KanbanTenant: KanbanTenant("project-test"),
		CreatedAt:    domainTestTime,
		WorktreeRoot: "/home/user/Projects/.hermoso-worktrees",
	}

	if err := project.Validate(); err != nil {
		t.Errorf("Validate should accept an absolute worktree_root: %v", err)
	}
}
