package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bocacorazon/hermoso/internal/testutil"
)

func TestDeterministicCreationReuseAndDirtyRejection(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	manager := newTestManager(t, repo)

	first, err := manager.Feature(context.Background(), "run-1", "feature-1", "main")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Feature(context.Background(), "run-1", "feature-1", "main")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("reused worktree = %#v, want %#v", second, first)
	}
	if first.ParentCommit != repo.Git("rev-parse", "main") {
		t.Errorf("parent commit = %q, want exact main commit", first.ParentCommit)
	}
	if first.Branch != "hermoso/run/run-1/feature/feature-1" || !strings.HasSuffix(first.Path, "/run-1/feature") {
		t.Errorf("non-deterministic feature location: %#v", first)
	}

	writeFile(t, first.Path, "dirty.txt", "dirty")
	_, err = manager.Feature(context.Background(), "run-1", "feature-1", "main")
	if !errors.Is(err, ErrDirty) {
		t.Fatalf("dirty reuse error = %v, want ErrDirty", err)
	}
}

func TestSameFeatureInParallelRunsHasIsolatedBranchesAndWorktrees(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	manager := newTestManager(t, repo)
	ctx := context.Background()

	first, err := manager.Feature(ctx, "run-1", "feature-1", "main")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Feature(ctx, "run-2", "feature-1", "main")
	if err != nil {
		t.Fatal(err)
	}
	if first.Branch == second.Branch || first.Path == second.Path {
		t.Fatalf("parallel run feature worktrees overlap: first=%#v second=%#v", first, second)
	}

	firstItem, _, err := manager.WorkItem(ctx, "run-1", "feature-1", "root", []Worktree{first})
	if err != nil {
		t.Fatal(err)
	}
	secondItem, _, err := manager.WorkItem(ctx, "run-2", "feature-1", "root", []Worktree{second})
	if err != nil {
		t.Fatal(err)
	}
	if firstItem.Branch == secondItem.Branch || firstItem.Path == secondItem.Path {
		t.Fatalf("parallel run item worktrees overlap: first=%#v second=%#v", firstItem, secondItem)
	}
	commitFile(t, firstItem.Path, "run.txt", "one\n", "run one")
	if _, err := os.Stat(filepath.Join(secondItem.Path, "run.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run-1 work leaked into run-2: %v", err)
	}
}

func TestWorkItemsSynchronizeFanOutAndFanInParents(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	manager := newTestManager(t, repo)
	ctx := context.Background()
	feature, err := manager.Feature(ctx, "run-1", "feature-1", "main")
	if err != nil {
		t.Fatal(err)
	}

	left, blocked, err := manager.WorkItem(ctx, "run-1", "feature-1", "left", []Worktree{feature})
	if err != nil || blocked != nil {
		t.Fatalf("create left: blocked=%#v err=%v", blocked, err)
	}
	right, blocked, err := manager.WorkItem(ctx, "run-1", "feature-1", "right", []Worktree{feature})
	if err != nil || blocked != nil {
		t.Fatalf("create right: blocked=%#v err=%v", blocked, err)
	}
	commitFile(t, left.Path, "left.txt", "left", "left")
	commitFile(t, right.Path, "right.txt", "right", "right")

	joined, blocked, err := manager.WorkItem(ctx, "run-1", "feature-1", "joined", []Worktree{right, left})
	if err != nil || blocked != nil {
		t.Fatalf("create joined: blocked=%#v err=%v", blocked, err)
	}
	for _, name := range []string{"left.txt", "right.txt"} {
		if _, err := os.Stat(filepath.Join(joined.Path, name)); err != nil {
			t.Errorf("joined worktree missing %s: %v", name, err)
		}
	}
	if joined.ParentBranch != right.Branch || joined.ParentCommit == "" {
		t.Errorf("parent recording = %q at %q, want exact requested first parent", joined.ParentBranch, joined.ParentCommit)
	}
}

func TestConflictCanContinueOrAbortWithoutResettingWork(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	manager := newTestManager(t, repo)
	ctx := context.Background()
	feature, err := manager.Feature(ctx, "run-1", "feature-1", "main")
	if err != nil {
		t.Fatal(err)
	}
	left, _, err := manager.WorkItem(ctx, "run-1", "feature-1", "left", []Worktree{feature})
	if err != nil {
		t.Fatal(err)
	}
	right, _, err := manager.WorkItem(ctx, "run-1", "feature-1", "right", []Worktree{feature})
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, left.Path, "README.md", "left\n", "left")
	commitFile(t, right.Path, "README.md", "right\n", "right")

	state, err := manager.Merge(ctx, left, right.Branch)
	if !errors.Is(err, ErrConflict) || !state.Conflicted || len(state.ConflictPaths) != 1 {
		t.Fatalf("merge state = %#v, err = %v; want explicit conflict", state, err)
	}
	originalConflict := readFile(t, left.Path, "README.md")
	writeFile(t, left.Path, "README.md", "edited resolution\n")
	if err := manager.AbortMerge(ctx, state); !errors.Is(err, ErrDirty) {
		t.Fatalf("abort edited conflict error = %v, want ErrDirty", err)
	}
	writeFile(t, left.Path, "README.md", originalConflict)
	if err := manager.AbortMerge(ctx, state); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, left.Path, "README.md"); got != "left\n" {
		t.Errorf("abort changed target work: %q", got)
	}

	state, err = manager.Merge(ctx, left, right.Branch)
	if !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	writeFile(t, left.Path, "README.md", "resolved\n")
	gitAt(t, left.Path, "add", "README.md")
	state, err = manager.ContinueMerge(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	if state.Conflicted || !gitSuccess(repo.Root, "merge-base", "--is-ancestor", right.Branch, left.Branch) {
		t.Errorf("continued merge did not integrate source: %#v", state)
	}
}

func TestSyntheticIntegrationNodesMergeLeavesAndResumeIdempotently(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	manager := newTestManager(t, repo)
	ctx := context.Background()
	feature, err := manager.Feature(ctx, "run-1", "feature-1", "main")
	if err != nil {
		t.Fatal(err)
	}

	var leaves []Worktree
	for _, id := range []string{"a", "b", "c", "d"} {
		leaf, _, err := manager.WorkItem(ctx, "run-1", "feature-1", id, []Worktree{feature})
		if err != nil {
			t.Fatal(err)
		}

		commitFile(t, leaf.Path, id+".txt", id+"\n", id)
		leaves = append(leaves, leaf)
	}

	result, err := manager.Integrate(ctx, "run-1", "feature-1", feature, leaves, []string{"test -f a.txt && test -f b.txt && test -f c.txt && test -f d.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 3 || len(result.Checks) != 1 {
		t.Fatalf("integration result = %#v, want 3 pairwise nodes and one check", result)
	}
	if result.Nodes[0].Left > result.Nodes[0].Right {
		t.Errorf("integration ordering is not deterministic: %#v", result.Nodes[0])
	}

	resumed, err := manager.Integrate(ctx, "run-1", "feature-1", feature, leaves, []string{"test -z \"$(git status --porcelain)\""})
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed.Nodes) != 0 || len(resumed.Checks) != 1 {
		t.Errorf("idempotent resume = %#v, want checks only", resumed)
	}
	blocked, err := manager.Integrate(ctx, "run-1", "feature-1", feature, leaves, []string{"echo baseline-failed && false"})
	if err == nil || blocked.Block == nil || blocked.Block.Kind != "baseline_check" || blocked.Block.Check == nil {
		t.Errorf("failed baseline result = %#v, err = %v; want explicit block state", blocked, err)
	}
}

func TestPathAndBranchValidationRejectEscapes(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	manager := newTestManager(t, repo)
	if _, err := manager.Feature(context.Background(), "../escape", "feature", "main"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("name escape error = %v, want ErrInvalidName", err)
	}

	if _, err := manager.Feature(context.Background(), "run", "feature", "--upload-pack=bad"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("branch injection error = %v, want ErrInvalidName", err)
	}

	outside := repo.Mkdir("outside")
	runPath := filepath.Join(manager.worktrees, "run")
	if err := os.MkdirAll(manager.worktrees, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, runPath); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Feature(context.Background(), "run", "feature", "main"); !errors.Is(err, ErrPathEscape) {
		t.Errorf("symlink escape error = %v, want ErrPathEscape", err)
	}
}

func TestValidateAllowedCommitAcceptsOnlyDirectAllowlistedPublication(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	manager := newTestManager(t, repo)
	feature, err := manager.Feature(context.Background(), "run-1", "feature-1", "main")
	if err != nil {
		t.Fatal(err)
	}
	parent, err := manager.CurrentCommit(feature)
	if err != nil {
		t.Fatal(err)
	}
	const path = "features/feature.feature"
	writeFile(t, feature.Path, path, "Feature: Verified behavior\n")
	publication, err := manager.CommitAllowedFiles(
		context.Background(), feature, []string{path}, "publish verified behavior",
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := manager.ValidateAllowedCommit(feature, parent, []string{path})
	if err != nil || got != publication {
		t.Fatalf("validated commit = %q, %v; want %q", got, err, publication)
	}

	commitFile(t, feature.Path, "unexpected.txt", "unexpected\n", "unexpected change")
	if _, err := manager.ValidateAllowedCommit(feature, parent, []string{path}); err == nil {
		t.Fatal("non-direct publication commit was accepted")
	}
}

func TestIntegrationRetryResumesConflictOnSecondSource(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	manager := newTestManager(t, repo)
	ctx := context.Background()
	feature, err := manager.Feature(ctx, "run-1", "feature-1", "main")
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := manager.WorkItem(ctx, "run-1", "feature-1", "a", []Worktree{feature})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := manager.WorkItem(ctx, "run-1", "feature-1", "b", []Worktree{feature})
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, first.Path, "README.md", "first\n", "a")
	commitFile(t, second.Path, "README.md", "second\n", "second")

	result, err := manager.Integrate(ctx, "run-1", "feature-1", feature, []Worktree{first, second}, nil)
	if !errors.Is(err, ErrConflict) || result.Merge == nil || result.Merge.SourceBranch != second.Branch {
		t.Fatalf("second-source conflict result=%#v err=%v", result, err)
	}
	writeFile(t, result.Merge.Worktree.Path, "README.md", "resolved\n")
	gitAt(t, result.Merge.Worktree.Path, "add", "README.md")

	result, err = manager.Integrate(ctx, "run-1", "feature-1", feature, []Worktree{first, second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Block != nil || !gitSuccess(repo.Root, "merge-base", "--is-ancestor", second.Branch, feature.Branch) {
		t.Fatalf("retry did not finish integration: %#v", result)
	}
}

func TestSymlinkedWorktreeRootIsRejectedWithoutExternalWrites(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	outside, err := os.MkdirTemp(filepath.Dir(repo.Root), "outside-worktrees-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(outside) })
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repo.Root, ".managed-worktrees")
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(repo.Root, root, "hermoso"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("manager error = %v, want ErrPathEscape", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "unchanged" {
		t.Fatalf("external sentinel changed: %q err=%v", got, err)
	}
}

func TestPreexistingDeterministicBranchIsNotReusedWithoutOwnership(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	repo.Git("branch", "hermoso/run/run-1/feature/feature-1")
	manager := newTestManager(t, repo)
	_, err := manager.Feature(context.Background(), "run-1", "feature-1", "main")
	if !errors.Is(err, ErrUnsafeReuse) {
		t.Fatalf("preexisting branch error = %v, want ErrUnsafeReuse", err)
	}
	if output := repo.Git("worktree", "list", "--porcelain"); strings.Contains(output, "/run-1/feature") {
		t.Errorf("unsafe branch was attached before validation:\n%s", output)
	}
}

func newTestManager(t *testing.T, repo *testutil.Repository) *Manager {
	t.Helper()
	manager, err := NewManager(repo.Root, filepath.Join(repo.Root, ".managed-worktrees"), "hermoso")
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func commitFile(t *testing.T, path, name, content, message string) {
	t.Helper()
	writeFile(t, path, name, content)
	gitAt(t, path, "add", "--all")
	gitAt(t, path, "commit", "--quiet", "-m", message)
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func gitAt(t *testing.T, path string, args ...string) string {
	t.Helper()
	output, err := git(path, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return output
}
