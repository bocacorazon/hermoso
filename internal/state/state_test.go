package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/testutil"
)

var stateTestTime = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

func TestDiscoverRepositoryCanonicalizesSubdirectory(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	repo.Git("remote", "add", "origin", repo.Root)
	subdirectory := repo.Mkdir("one/two")
	discovered, err := DiscoverRepository(context.Background(), subdirectory)
	if err != nil {
		t.Fatal(err)
	}

	root, err := filepath.EvalSymlinks(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if discovered.Root != root || discovered.Target.Repository != root {
		t.Errorf("repository root = %q, want %q", discovered.Root, root)
	}
	if !filepath.IsAbs(discovered.ExcludePath) {
		t.Errorf("exclude path = %q, want absolute", discovered.ExcludePath)
	}
	if !strings.HasPrefix(discovered.Target.RemoteURL, "file://") {
		t.Errorf("remote URL = %q, want normalized file URL", discovered.Target.RemoteURL)
	}
}

func TestDiscoverRepositoryRequiresExplicitPath(t *testing.T) {
	t.Parallel()
	if _, err := DiscoverRepository(context.Background(), ""); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("error = %v, want ErrNotRepository", err)
	}
}

func TestInitializeIsIdempotentAndRegistersExcludeOnce(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	first, created, err := Initialize(context.Background(), repo.Root, stateTestTime)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first initialization did not create project state")
	}
	second, created, err := Initialize(context.Background(), repo.Mkdir("nested"), stateTestTime.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if created || second != first {
		t.Errorf("second initialization = (%#v, %t), want existing %#v", second, created, first)
	}
	exclude, err := os.ReadFile(filepath.Join(repo.Root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(exclude), excludeEntry); count != 1 {
		t.Errorf("exclude entry count = %d, want 1:\n%s", count, exclude)
	}
	if status := repo.Git("status", "--porcelain"); status != "" {
		t.Errorf("repository is dirty after initialization:\n%s", status)
	}
}

func TestLoadReportsCorruptAndIncompatibleState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    error
	}{
		{name: "corrupt JSON", content: "{", want: ErrCorruptState},
		{name: "missing version", content: `{}`, want: ErrCorruptState},
		{name: "unknown field", content: `{"schema_version":"1","surprise":true}`, want: ErrCorruptState},
		{name: "incompatible version", content: `{"schema_version":"2"}`, want: ErrIncompatibleState},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repo := testutil.NewRepository(t)
			repo.Write(".hermoso/state.lock", "")
			repo.Write(".hermoso/project.json", test.content)
			_, _, err := Load(context.Background(), repo.Root)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is(%v)", err, test.want)
			}
		})
	}
}

func TestLoadUninitializedDoesNotModifyRepository(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	_, _, err := Load(context.Background(), repo.Root)
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("error = %v, want ErrNotInitialized", err)
	}

	if _, err := os.Stat(filepath.Join(repo.Root, ".hermoso")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("status probe created state directory: %v", err)
	}

	if status := repo.Git("status", "--porcelain"); status != "" {
		t.Errorf("repository is dirty after status probe:\n%s", status)
	}

}

func TestSymlinkedStateRootIsRejectedWithoutExternalWrites(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	outside, err := os.MkdirTemp(filepath.Dir(repo.Root), "outside-state-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(outside) })
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo.Root, ".hermoso")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Initialize(context.Background(), repo.Root, stateTestTime); !errors.Is(err, ErrUnsafeStatePath) {
		t.Fatalf("initialize error = %v, want ErrUnsafeStatePath", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "unchanged" {
		t.Fatalf("external sentinel changed: %q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "project.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state escaped through symlink: %v", err)
	}
}

func TestLoadRunIdentitySurvivesCurrentBranchChange(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	if _, _, err := Initialize(context.Background(), repo.Root, stateTestTime); err != nil {
		t.Fatal(err)
	}
	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartRun(context.Background(), "feature-1", stateTestTime)
	if err != nil {
		t.Fatal(err)
	}

	repo.Git("checkout", "-b", "side-branch")

	loadedStore, status, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatalf("load after branch switch: %v", err)
	}
	if len(status.Runs) != 1 || !status.Runs[0].Context.Equal(run.Context) {
		t.Fatalf("runs after branch switch = %#v, want context %#v", status.Runs, run.Context)
	}
	if _, err := loadedStore.Context(context.Background(), run.Context.RunID); err != nil {
		t.Fatalf("resolve context after branch switch: %v", err)
	}
}

func TestConcurrentRunUpdatesAreSerialized(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	_, _, err := Initialize(context.Background(), repo.Root, stateTestTime)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartRun(context.Background(), "feature-1", stateTestTime)
	if err != nil {
		t.Fatal(err)
	}

	const updates = 20
	var wait sync.WaitGroup
	errorsFound := make(chan error, updates)
	for i := 0; i < updates; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.UpdateRun(context.Background(), run.Context, func(current *domain.Run) error {
				current.Revision++
				current.UpdatedAt = current.UpdatedAt.Add(time.Second)
				return nil
			})
			errorsFound <- err
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Errorf("update run: %v", err)
		}
	}

	_, status, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if got := status.Runs[0].Revision; got != updates+1 {
		t.Errorf("revision = %d, want %d", got, updates+1)
	}
}

func TestUpdateRunRejectsIllegalLifecycleTransition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	if _, _, err := Initialize(context.Background(), repo.Root, stateTestTime); err != nil {
		t.Fatal(err)
	}
	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartRun(context.Background(), "feature-1", stateTestTime)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.UpdateRun(context.Background(), run.Context, func(current *domain.Run) error {
		current.Phase = domain.PhaseRelease
		current.Status = domain.StatusReleased
		return nil
	}); err == nil || !strings.Contains(err.Error(), "illegal lifecycle transition") {
		t.Fatalf("error = %v, want illegal lifecycle transition", err)
	}
}

func TestBindTaskIsIdempotentAndRejectsConflicts(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	if _, _, err := Initialize(context.Background(), repo.Root, stateTestTime); err != nil {
		t.Fatal(err)
	}
	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartRun(context.Background(), "feature-1", stateTestTime)
	if err != nil {
		t.Fatal(err)
	}

	first, created, err := store.BindTask(context.Background(), run.Context, "api", "hermes-101", stateTestTime.Add(time.Minute))
	if err != nil || !created {
		t.Fatalf("first binding = (%#v, %t, %v)", first, created, err)
	}
	repeated, created, err := store.BindTask(context.Background(), run.Context, "api", "hermes-101", stateTestTime.Add(time.Hour))
	if err != nil || created || repeated != first {
		t.Fatalf("repeat = (%#v, %t, %v), want (%#v, false, nil)", repeated, created, err, first)
	}
	if _, _, err := store.BindTask(context.Background(), run.Context, "api", "hermes-102", stateTestTime.Add(time.Hour)); !errors.Is(err, ErrBindingConflict) {
		t.Errorf("work item rebind error = %v, want ErrBindingConflict", err)
	}
	if _, _, err := store.BindTask(context.Background(), run.Context, "ui", "hermes-101", stateTestTime.Add(time.Hour)); !errors.Is(err, ErrBindingConflict) {
		t.Errorf("task reuse error = %v, want ErrBindingConflict", err)
	}

	_, status, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	got := status.Runs[0]
	if len(got.TaskBindings) != 1 || got.TaskBindings[0] != first {
		t.Errorf("persisted bindings = %#v, want %#v", got.TaskBindings, first)
	}
	if got.Revision != 2 {
		t.Errorf("revision = %d, want 2", got.Revision)
	}
}

func TestStateOperationsRejectMismatchedContext(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	if _, _, err := Initialize(context.Background(), repo.Root, stateTestTime); err != nil {
		t.Fatal(err)
	}
	store, _, err := Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartRun(context.Background(), "feature-1", stateTestTime)
	if err != nil {
		t.Fatal(err)
	}
	mismatch := run.Context
	mismatch.FeatureID = "feature-2"
	if _, err := store.UpdateRun(context.Background(), mismatch, func(*domain.Run) error { return nil }); !errors.Is(err, ErrIncompatibleState) {
		t.Fatalf("update error = %v, want ErrIncompatibleState", err)
	}
	if _, _, err := store.BindTask(context.Background(), mismatch, "api", "task-1", stateTestTime); !errors.Is(err, ErrIncompatibleState) {
		t.Fatalf("binding error = %v, want ErrIncompatibleState", err)
	}
}
