package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bocacorazon/hermoso/internal/testutil"
)

func TestInitializeStoresProfilePath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	profileFile := filepath.Join(repo.Root, "profile.json")
	repo.Write("profile.json", `{"dispatch":{"default_runtime_budget_seconds":300}}`)

	project, created, err := InitializeWithProfile(context.Background(), repo.Root, profileFile, stateTestTime)
	if err != nil {
		t.Fatalf("InitializeWithProfile: %v", err)
	}
	if !created {
		t.Fatal("expected project to be created")
	}
	if project.ProfilePath != profileFile {
		t.Errorf("project.ProfilePath = %q, want %q", project.ProfilePath, profileFile)
	}

	// Verify it's persisted on disk
	_, status, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if status.Project.ProfilePath != profileFile {
		t.Errorf("loaded project.ProfilePath = %q, want %q", status.Project.ProfilePath, profileFile)
	}
}

func TestInitializeWithoutProfileLeavesPathEmpty(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)

	project, _, err := Initialize(context.Background(), repo.Root, stateTestTime)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if project.ProfilePath != "" {
		t.Errorf("project.ProfilePath = %q, want empty", project.ProfilePath)
	}
}

func TestStartRunStoresProfilePath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	profileFile := filepath.Join(repo.Root, "profile.json")
	repo.Write("profile.json", `{"dispatch":{"default_runtime_budget_seconds":300}}`)

	_, _, err := InitializeWithProfile(context.Background(), repo.Root, profileFile, stateTestTime)
	if err != nil {
		t.Fatalf("InitializeWithProfile: %v", err)
	}

	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	runOverride := filepath.Join(repo.Root, "override.json")
	run, err := store.StartRunWithProfile(context.Background(), "feature-1", runOverride, stateTestTime)
	if err != nil {
		t.Fatalf("StartRunWithProfile: %v", err)
	}
	if run.ProfilePath != runOverride {
		t.Errorf("run.ProfilePath = %q, want %q", run.ProfilePath, runOverride)
	}

	// Verify persistence
	loaded, err := store.Run(context.Background(), run.Context)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if loaded.ProfilePath != runOverride {
		t.Errorf("loaded run.ProfilePath = %q, want %q", loaded.ProfilePath, runOverride)
	}
}

func TestStartRunWithoutProfileLeavesPathEmpty(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	_, _, err := Initialize(context.Background(), repo.Root, stateTestTime)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	run, err := store.StartRun(context.Background(), "feature-1", stateTestTime)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.ProfilePath != "" {
		t.Errorf("run.ProfilePath = %q, want empty", run.ProfilePath)
	}
}

func TestResolveProfileReturnsRunLevelWhenSet(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	projectProfile := filepath.Join(repo.Root, "project-profile.json")
	runProfile := filepath.Join(repo.Root, "run-profile.json")
	repo.Write("project-profile.json", `{"dispatch":{"default_runtime_budget_seconds":300}}`)
	repo.Write("run-profile.json", `{"dispatch":{"default_runtime_budget_seconds":300}}`)

	_, _, err := InitializeWithProfile(context.Background(), repo.Root, projectProfile, stateTestTime)
	if err != nil {
		t.Fatalf("InitializeWithProfile: %v", err)
	}
	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	run, err := store.StartRunWithProfile(context.Background(), "feature-1", runProfile, stateTestTime)
	if err != nil {
		t.Fatalf("StartRunWithProfile: %v", err)
	}

	resolved, err := store.ResolveProfile(context.Background(), run.Context)
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if resolved != runProfile {
		t.Errorf("resolved = %q, want run-level %q", resolved, runProfile)
	}
}

func TestResolveProfileFallsBackToProjectLevel(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	projectProfile := filepath.Join(repo.Root, "project-profile.json")
	repo.Write("project-profile.json", `{"dispatch":{"default_runtime_budget_seconds":300}}`)

	_, _, err := InitializeWithProfile(context.Background(), repo.Root, projectProfile, stateTestTime)
	if err != nil {
		t.Fatalf("InitializeWithProfile: %v", err)
	}
	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	run, err := store.StartRun(context.Background(), "feature-1", stateTestTime)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	resolved, err := store.ResolveProfile(context.Background(), run.Context)
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if resolved != projectProfile {
		t.Errorf("resolved = %q, want project-level %q", resolved, projectProfile)
	}
}

func TestResolveProfileErrorsWhenNeitherSet(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	_, _, err := Initialize(context.Background(), repo.Root, stateTestTime)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	run, err := store.StartRun(context.Background(), "feature-1", stateTestTime)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	_, err = store.ResolveProfile(context.Background(), run.Context)
	if err == nil {
		t.Fatal("expected error when neither profile is set, got nil")
	}
}
