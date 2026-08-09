package state

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bocacorazon/hermoso/internal/domain"
)

const excludeEntry = "/.hermoso/"

type Store struct {
	repository Repository
}

type Status struct {
	Project domain.Project `json:"project"`
	Runs    []domain.Run   `json:"runs"`
}

func Open(repository Repository) Store {
	return Store{repository: repository}
}

func (s Store) Repository() Repository {
	return s.repository
}

func Initialize(ctx context.Context, start string, now time.Time) (domain.Project, bool, error) {
	repository, err := DiscoverRepository(ctx, start)
	if err != nil {
		return domain.Project{}, false, err
	}
	store := Open(repository)
	var project domain.Project
	created := false
	err = store.withLock(ctx, true, func() error {
		existing, err := store.readProjectUnlocked()
		switch {
		case err == nil:
			project = existing
		case !errors.Is(err, ErrNotInitialized):
			return err
		default:
			projectID, err := newID("project")
			if err != nil {
				return err
			}
			project = domain.Project{
				SchemaVersion: domain.SchemaVersion,
				ProjectID:     projectID,
				Target:        repository.Target,
				KanbanTenant:  domain.KanbanTenant(projectID),
				CreatedAt:     now.UTC(),
			}
			if err := project.Validate(); err != nil {
				return fmt.Errorf("create project state: %w", err)
			}
			if err := writeJSONAtomic(store.repository.Root, store.projectPath(), project); err != nil {
				return err
			}
			created = true
		}
		return registerExclude(repository.ExcludePath)
	})
	return project, created, err
}

func Load(ctx context.Context, start string) (Store, Status, error) {
	repository, err := DiscoverRepository(ctx, start)
	if err != nil {
		return Store{}, Status{}, err
	}
	store := Open(repository)
	var status Status
	err = store.withLock(ctx, false, func() error {
		project, err := store.readProjectUnlocked()
		if err != nil {
			return err
		}
		runs, err := store.readRunsUnlocked(project)
		if err != nil {
			return err
		}
		status = Status{Project: project, Runs: runs}
		return nil
	})
	return store, status, err
}

func (s Store) StartRun(ctx context.Context, featureID string, now time.Time) (domain.Run, error) {
	var run domain.Run
	err := s.withLock(ctx, true, func() error {
		project, err := s.readProjectUnlocked()
		if err != nil {
			return err
		}
		runID, err := newID("run")
		if err != nil {
			return err
		}
		run = domain.Run{
			SchemaVersion: domain.SchemaVersion,
			Context: domain.ContextRef{
				SchemaVersion: domain.ContextSchemaVersion,
				ProjectID:     project.ProjectID,
				FeatureID:     featureID,
				RunID:         runID,
				Repository:    project.Target,
			},
			Phase:     domain.PhaseDesign,
			Status:    domain.StatusPending,
			Revision:  1,
			CreatedAt: now.UTC(),
			UpdatedAt: now.UTC(),
		}
		if err := run.Validate(); err != nil {
			return err
		}
		return writeJSONAtomic(s.repository.Root, s.runPath(run.Context.RunID), run)
	})
	return run, err
}

func (s Store) Context(ctx context.Context, runID string) (domain.ContextRef, error) {
	var result domain.ContextRef
	err := s.withLock(ctx, false, func() error {
		project, err := s.readProjectUnlocked()
		if err != nil {
			return err
		}
		var run domain.Run
		if err := readStateJSON(s.runPath(runID), "run", &run); err != nil {
			return err
		}
		if err := validateRunIdentity(project, runID, run); err != nil {
			return err
		}
		result = run.Context
		return nil
	})
	return result, err
}

func (s Store) Run(ctx context.Context, expected domain.ContextRef) (domain.Run, error) {
	var run domain.Run
	err := s.withLock(ctx, false, func() error {
		project, err := s.readProjectUnlocked()
		if err != nil {
			return err
		}
		if err := readStateJSON(s.runPath(expected.RunID), "run", &run); err != nil {
			return err
		}
		if err := validateRunIdentity(project, expected.RunID, run); err != nil {
			return err
		}
		if !run.Context.Equal(expected) {
			return fmt.Errorf("%w: supplied execution context does not match persisted run", ErrIncompatibleState)
		}
		return nil
	})
	return run, err
}

func (s Store) UpdateRun(ctx context.Context, expected domain.ContextRef, update func(*domain.Run) error) (domain.Run, error) {
	if err := expected.Validate(); err != nil {
		return domain.Run{}, fmt.Errorf("invalid execution context: %w", err)
	}
	var run domain.Run
	err := s.withLock(ctx, true, func() error {
		project, err := s.readProjectUnlocked()
		if err != nil {
			return err
		}
		if err := readStateJSON(s.runPath(expected.RunID), "run", &run); err != nil {
			return err
		}
		if err := validateRunIdentity(project, expected.RunID, run); err != nil {
			return err
		}
		if !run.Context.Equal(expected) {
			return fmt.Errorf("%w: supplied execution context does not match persisted run", ErrIncompatibleState)
		}
		previousPhase, previousStatus := run.Phase, run.Status
		if err := update(&run); err != nil {
			return err
		}
		if err := domain.ValidateTransition(previousPhase, previousStatus, run.Phase, run.Status); err != nil {
			return err
		}
		if err := run.Validate(); err != nil {
			return fmt.Errorf("%w: run %q: %v", ErrCorruptState, expected.RunID, err)
		}
		return writeJSONAtomic(s.repository.Root, s.runPath(expected.RunID), run)
	})
	return run, err
}

// BindTask records the Hermes task returned for a work item without consulting
// Hermes storage. Repeating the same binding is idempotent; changing either
// side of an existing binding is rejected.
func (s Store) BindTask(ctx context.Context, execution domain.ContextRef, workItemID, taskID string, now time.Time) (domain.TaskBinding, bool, error) {
	candidate := domain.TaskBinding{Context: execution, WorkItemID: workItemID, TaskID: taskID, BoundAt: now.UTC()}
	if err := candidate.Validate(); err != nil {
		return domain.TaskBinding{}, false, err
	}
	var result domain.TaskBinding
	created := false
	_, err := s.UpdateRun(ctx, execution, func(run *domain.Run) error {
		for _, binding := range run.TaskBindings {
			if binding.WorkItemID == workItemID {
				if binding.TaskID != taskID {
					return fmt.Errorf("%w: work item %q is already bound to task %q", ErrBindingConflict, workItemID, binding.TaskID)
				}
				result = binding
				return nil
			}
			if binding.TaskID == taskID {
				return fmt.Errorf("%w: task %q is already bound to work item %q", ErrBindingConflict, taskID, binding.WorkItemID)
			}
		}
		run.TaskBindings = append(run.TaskBindings, candidate)
		run.Revision++
		if candidate.BoundAt.After(run.UpdatedAt) {
			run.UpdatedAt = candidate.BoundAt
		}
		result = candidate
		created = true
		return nil
	})
	return result, created, err
}

func (s Store) stateDir() string {
	return filepath.Join(s.repository.Root, ".hermoso")
}

func (s Store) projectPath() string {
	return filepath.Join(s.stateDir(), "project.json")
}

func (s Store) runPath(runID string) string {
	return filepath.Join(s.stateDir(), "runs", runID, "run.json")
}

func (s Store) readProjectUnlocked() (domain.Project, error) {
	var project domain.Project
	if err := readStateJSON(s.projectPath(), "project", &project); err != nil {
		return domain.Project{}, err
	}
	if project.Target.Repository != s.repository.Root {
		return domain.Project{}, fmt.Errorf("%w: project targets %q, repository is %q", ErrIncompatibleState, project.Target.Repository, s.repository.Root)
	}
	return project, nil
}

func (s Store) readRunsUnlocked(project domain.Project) ([]domain.Run, error) {
	entries, err := os.ReadDir(filepath.Join(s.stateDir(), "runs"))
	if errors.Is(err, os.ErrNotExist) {
		return []domain.Run{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read run state: %w", err)
	}
	runs := make([]domain.Run, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var run domain.Run
		if err := readStateJSON(s.runPath(entry.Name()), "run", &run); err != nil {
			return nil, err
		}
		if run.Context.RunID != entry.Name() || run.Context.ProjectID != project.ProjectID {
			return nil, fmt.Errorf("%w: run directory %q has inconsistent identity", ErrCorruptState, entry.Name())
		}
		if !run.Context.Repository.Equal(project.Target) {
			return nil, fmt.Errorf("%w: run directory %q targets a different repository", ErrIncompatibleState, entry.Name())
		}
		runs = append(runs, run)
	}

	return runs, nil
}

func validateRunIdentity(project domain.Project, runID string, run domain.Run) error {
	if run.Context.RunID != runID || run.Context.ProjectID != project.ProjectID {
		return fmt.Errorf("%w: run %q does not belong to this project", ErrCorruptState, runID)
	}
	if !run.Context.Repository.Equal(project.Target) {
		return fmt.Errorf("%w: run %q repository identity does not match project", ErrIncompatibleState, runID)
	}
	return nil
}

func (s Store) withLock(ctx context.Context, exclusive bool, fn func() error) error {
	if err := verifyStateTree(s.repository.Root, s.stateDir()); err != nil {
		return err
	}
	if exclusive {
		if err := os.MkdirAll(s.stateDir(), 0o700); err != nil {
			return fmt.Errorf("create state directory: %w", err)
		}
	} else {
		if _, err := os.Stat(s.stateDir()); errors.Is(err, os.ErrNotExist) {
			return ErrNotInitialized
		} else if err != nil {
			return fmt.Errorf("inspect state directory: %w", err)
		}
		if err := verifyStateTree(s.repository.Root, s.stateDir()); err != nil {
			return err
		}
	}
	flags := os.O_RDWR
	if exclusive {
		flags |= os.O_CREATE
	}
	lock, err := os.OpenFile(filepath.Join(s.stateDir(), "state.lock"), flags, 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: state lock is missing", ErrCorruptState)
	}
	if err != nil {
		return fmt.Errorf("open state lock: %w", err)
	}
	defer lock.Close()

	operation := syscall.LOCK_SH
	if exclusive {
		operation = syscall.LOCK_EX
	}
	for {
		err = syscall.Flock(int(lock.Fd()), operation|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return fmt.Errorf("acquire state lock: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("acquire state lock: %w", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}

func readStateJSON(path, kind string, target interface{ Validate() error }) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && kind == "project" {
		return ErrNotInitialized
	}
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: missing %s state %q", ErrCorruptState, kind, path)
	}
	if err != nil {
		return fmt.Errorf("read %s state: %w", kind, err)
	}

	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("%w: decode %s state %q: %v", ErrCorruptState, kind, path, err)
	}
	if envelope.SchemaVersion != "" && envelope.SchemaVersion != domain.SchemaVersion {
		return fmt.Errorf("%w: %s state %q uses schema version %q, want %q", ErrIncompatibleState, kind, path, envelope.SchemaVersion, domain.SchemaVersion)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: decode %s state %q: %v", ErrCorruptState, kind, path, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return fmt.Errorf("%w: decode %s state %q: %v", ErrCorruptState, kind, path, err)
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("%w: %s state %q: %v", ErrCorruptState, kind, path, err)
	}
	return nil
}

func writeJSONAtomic(repositoryRoot, path string, value any) error {
	if err := verifyContainedStatePath(repositoryRoot, path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state parent: %w", err)
	}
	if err := verifyContainedStatePath(repositoryRoot, path); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return fmt.Errorf("create atomic state file: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("set state permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	parent, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open state parent for sync: %w", err)
	}
	defer parent.Close()
	if err := parent.Sync(); err != nil {
		return fmt.Errorf("sync state parent: %w", err)
	}
	return nil
}

func verifyStateTree(repositoryRoot, stateDir string) error {
	if err := verifyContainedStatePath(repositoryRoot, stateDir); err != nil {
		return err
	}
	info, err := os.Lstat(stateDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect state directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: symlink is not allowed: %s", ErrUnsafeStatePath, stateDir)
	}
	for _, path := range []string{
		filepath.Join(stateDir, "project.json"),
		filepath.Join(stateDir, "state.lock"),
	} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect state path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink is not allowed: %s", ErrUnsafeStatePath, path)
		}
	}
	runsDir := filepath.Join(stateDir, "runs")
	if _, err := os.Lstat(runsDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect run state directory: %w", err)
	}
	return filepath.WalkDir(runsDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink is not allowed: %s", ErrUnsafeStatePath, path)
		}
		return nil
	})
}

func verifyContainedStatePath(repositoryRoot, path string) error {
	root := filepath.Clean(repositoryRoot)
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolute = filepath.Clean(absolute)
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("%w: %s escapes repository %s", ErrUnsafeStatePath, absolute, root)
	}
	for probe := absolute; probe != root; probe = filepath.Dir(probe) {
		info, err := os.Lstat(probe)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink is not allowed: %s", ErrUnsafeStatePath, probe)
		}
	}
	return nil
}

func registerExclude(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read Git exclude file: %w", err)
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if strings.TrimSpace(string(line)) == excludeEntry {
			return nil
		}
	}
	if len(data) != 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, excludeEntry...)
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Git exclude directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("update Git exclude file: %w", err)
	}
	return nil
}

func newID(prefix string) (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate %s ID: %w", prefix, err)
	}
	return prefix + "-" + hex.EncodeToString(random[:]), nil
}
