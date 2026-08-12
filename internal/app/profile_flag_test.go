package app

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/bocacorazon/hermoso/internal/state"
	"github.com/bocacorazon/hermoso/internal/testutil"
)

func TestRunInitWithProfileFlag(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	profilePath := filepath.Join(repo.Root, "my-profile.json")
	repo.Write("my-profile.json", `{"dispatch":{"default_runtime_budget_seconds":300}}`)

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"init", repo.Root, "--profile", profilePath, "--json"}, testDependencies(&stdout, &stderr))
	if exitCode != ExitOK {
		t.Fatalf("init exit code = %d, stderr = %q", exitCode, stderr.String())
	}

	// Verify project.json has the profile path
	_, status, err := state.Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if status.Project.ProfilePath != profilePath {
		t.Errorf("project.ProfilePath = %q, want %q", status.Project.ProfilePath, profilePath)
	}
}

func TestRunInitWithoutProfileFlag(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"init", repo.Root, "--json"}, testDependencies(&stdout, &stderr))
	if exitCode != ExitOK {
		t.Fatalf("init exit code = %d, stderr = %q", exitCode, stderr.String())
	}

	_, status, err := state.Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if status.Project.ProfilePath != "" {
		t.Errorf("project.ProfilePath = %q, want empty", status.Project.ProfilePath)
	}
}

func TestRunStartWithProfileFlag(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	repo.Write("profile.json", `{"dispatch":{"default_runtime_budget_seconds":300}}`)

	var initOut, initErr bytes.Buffer
	Run(context.Background(), []string{"init", repo.Root, "--json"}, testDependencies(&initOut, &initErr))

	runProfile := filepath.Join(repo.Root, "run-profile.json")
	repo.Write("run-profile.json", `{"dispatch":{"default_runtime_budget_seconds":300}}`)

	var startOut, startErr bytes.Buffer
	exitCode := Run(context.Background(), []string{"start", "feature-1", repo.Root, "--profile", runProfile, "--json"}, testDependencies(&startOut, &startErr))
	if exitCode != ExitOK {
		t.Fatalf("start exit code = %d, stderr = %q", exitCode, startErr.String())
	}

	var started response
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	run := started.Data["run"].(map[string]any)
	if run["profile_path"] != runProfile {
		t.Errorf("run.profile_path = %v, want %q", run["profile_path"], runProfile)
	}

	// Verify persistence
	_, status, err := state.Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(status.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(status.Runs))
	}
	if status.Runs[0].ProfilePath != runProfile {
		t.Errorf("persisted run.ProfilePath = %q, want %q", status.Runs[0].ProfilePath, runProfile)
	}
}

func TestRunStartWithoutProfileFlag(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)

	var initOut, initErr bytes.Buffer
	Run(context.Background(), []string{"init", repo.Root, "--json"}, testDependencies(&initOut, &initErr))

	var startOut, startErr bytes.Buffer
	exitCode := Run(context.Background(), []string{"start", "feature-1", repo.Root, "--json"}, testDependencies(&startOut, &startErr))
	if exitCode != ExitOK {
		t.Fatalf("start exit code = %d, stderr = %q", exitCode, startErr.String())
	}

	var started response
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	run := started.Data["run"].(map[string]any)
	if _, present := run["profile_path"]; present {
		t.Errorf("run.profile_path should be omitted when empty, got %v", run["profile_path"])
	}
}
