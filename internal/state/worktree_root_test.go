package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bocacorazon/hermoso/internal/testutil"
)

func openTestStore(t *testing.T, root string) Store {
	t.Helper()
	repository, err := DiscoverRepository(context.Background(), root)
	if err != nil {
		t.Fatalf("DiscoverRepository: %v", err)
	}
	return Open(repository)
}

func TestSetWorktreeRootPersists(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	if _, _, err := Initialize(context.Background(), repo.Root, stateTestTime); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	store := openTestStore(t, repo.Root)
	root := "/home/user/Projects/.hermoso-worktrees"
	project, err := store.SetWorktreeRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("SetWorktreeRoot: %v", err)
	}
	if project.WorktreeRoot != root {
		t.Errorf("project.WorktreeRoot = %q, want %q", project.WorktreeRoot, root)
	}

	_, status, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if status.Project.WorktreeRoot != root {
		t.Errorf("loaded project.WorktreeRoot = %q, want %q", status.Project.WorktreeRoot, root)
	}
}

func TestSetWorktreeRootClearsWithEmpty(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	if _, _, err := Initialize(context.Background(), repo.Root, stateTestTime); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	store := openTestStore(t, repo.Root)
	if _, err := store.SetWorktreeRoot(context.Background(), "/home/user/Projects/.hermoso-worktrees"); err != nil {
		t.Fatalf("SetWorktreeRoot: %v", err)
	}

	project, err := store.SetWorktreeRoot(context.Background(), "")
	if err != nil {
		t.Fatalf("SetWorktreeRoot clear: %v", err)
	}
	if project.WorktreeRoot != "" {
		t.Errorf("project.WorktreeRoot = %q, want empty", project.WorktreeRoot)
	}

	projectJSON, err := os.ReadFile(filepath.Join(repo.Root, ".hermoso", "project.json"))
	if err != nil {
		t.Fatalf("read project.json: %v", err)
	}
	if strings.Contains(string(projectJSON), "worktree_root") {
		t.Errorf("worktree_root should be omitted after clearing, got: %s", projectJSON)
	}
}

func TestSetWorktreeRootRejectsRelative(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	if _, _, err := Initialize(context.Background(), repo.Root, stateTestTime); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	store := openTestStore(t, repo.Root)
	if _, err := store.SetWorktreeRoot(context.Background(), "relative/path"); err == nil {
		t.Fatal("SetWorktreeRoot should reject a relative path")
	}
}

func TestSetWorktreeRootRequiresInitializedProject(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	store := openTestStore(t, repo.Root)
	if _, err := store.SetWorktreeRoot(context.Background(), "/home/user/Projects/.hermoso-worktrees"); err == nil {
		t.Fatal("SetWorktreeRoot should fail on an uninitialized project")
	}
}

func TestSetWorktreeRootLeavesOtherFieldsIntact(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	profileFile := filepath.Join(repo.Root, "profile.json")
	repo.Write("profile.json", `{"dispatch":{"default_runtime_budget_seconds":300}}`)
	if _, _, err := InitializeWithProfile(context.Background(), repo.Root, profileFile, stateTestTime); err != nil {
		t.Fatalf("InitializeWithProfile: %v", err)
	}

	store := openTestStore(t, repo.Root)
	project, err := store.SetWorktreeRoot(context.Background(), "/home/user/Projects/.hermoso-worktrees")
	if err != nil {
		t.Fatalf("SetWorktreeRoot: %v", err)
	}
	if project.ProfilePath != profileFile {
		t.Errorf("project.ProfilePath = %q, want %q", project.ProfilePath, profileFile)
	}
	if project.ProjectID == "" {
		t.Error("project.ProjectID should be preserved")
	}
}
