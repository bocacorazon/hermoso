package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bocacorazon/hermoso/internal/testutil"
)

func TestWorktreeRootCommandSetAndGet(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	var out, errBuf bytes.Buffer
	if code := Run(context.Background(), []string{"init", repo.Root, "--json"}, testDependencies(&out, &errBuf)); code != ExitOK {
		t.Fatalf("init exit code = %d, stderr = %q", code, errBuf.String())
	}

	// Get before set: reports the in-repo default.
	out.Reset()
	errBuf.Reset()
	if code := Run(context.Background(), []string{"worktree-root", repo.Root, "--json"}, testDependencies(&out, &errBuf)); code != ExitOK {
		t.Fatalf("worktree-root get exit code = %d, stderr = %q", code, errBuf.String())
	}
	var initial response
	if err := json.Unmarshal(out.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	wantDefault := filepath.Join(repo.Root, ".hermoso", "worktrees")
	if initial.Data["worktree_root"] != wantDefault {
		t.Errorf("initial worktree_root = %v, want %q", initial.Data["worktree_root"], wantDefault)
	}

	// Set: persists to project state.
	root := "/tmp/hermoso-test-worktrees"
	out.Reset()
	errBuf.Reset()
	if code := Run(context.Background(), []string{"worktree-root", repo.Root, root, "--json"}, testDependencies(&out, &errBuf)); code != ExitOK {
		t.Fatalf("worktree-root set exit code = %d, stderr = %q", code, errBuf.String())
	}
	var set response
	if err := json.Unmarshal(out.Bytes(), &set); err != nil {
		t.Fatal(err)
	}
	if set.Data["worktree_root"] != root {
		t.Errorf("set worktree_root = %v, want %q", set.Data["worktree_root"], root)
	}

	// Project state on disk carries the override.
	projectJSON, err := os.ReadFile(filepath.Join(repo.Root, ".hermoso", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(projectJSON), `"worktree_root": "`+root+`"`) {
		t.Errorf("project.json should carry worktree_root %q, got: %s", root, projectJSON)
	}

	// Get after set: reports the override.
	out.Reset()
	errBuf.Reset()
	if code := Run(context.Background(), []string{"worktree-root", repo.Root, "--json"}, testDependencies(&out, &errBuf)); code != ExitOK {
		t.Fatalf("worktree-root get exit code = %d, stderr = %q", code, errBuf.String())
	}
	var after response
	if err := json.Unmarshal(out.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.Data["worktree_root"] != root {
		t.Errorf("after set worktree_root = %v, want %q", after.Data["worktree_root"], root)
	}
}

func TestWorktreeRootCommandRejectsRelativePath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	var out, errBuf bytes.Buffer
	if code := Run(context.Background(), []string{"init", repo.Root, "--json"}, testDependencies(&out, &errBuf)); code != ExitOK {
		t.Fatalf("init exit code = %d, stderr = %q", code, errBuf.String())
	}

	out.Reset()
	errBuf.Reset()
	if code := Run(context.Background(), []string{"worktree-root", repo.Root, "relative/path", "--json"}, testDependencies(&out, &errBuf)); code == ExitOK {
		t.Fatal("worktree-root set should reject a relative path")
	}
}
