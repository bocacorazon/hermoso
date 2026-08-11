package state

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func (s Store) PutModelSnapshot(ctx context.Context, candidate domain.ModelSnapshot) (domain.ModelSnapshot, bool, error) {
	snapshot, err := domain.FinalizeModelSnapshot(candidate)
	if err != nil {
		return domain.ModelSnapshot{}, false, fmt.Errorf("finalize repository model: %w", err)
	}
	changed := false
	err = s.withLock(ctx, true, func() error {
		project, err := s.readProjectUnlocked()
		if err != nil {
			return err
		}
		if snapshot.Manifest.ProjectID != project.ProjectID || !snapshot.Manifest.Repository.Equal(project.Target) {
			return fmt.Errorf("%w: repository model identity does not match project", ErrIncompatibleState)
		}
		if err := s.verifyModelTree(); err != nil {
			return err
		}

		finalDir, err := s.modelSnapshotPath(snapshot.Manifest.SnapshotID)
		if err != nil {
			return err
		}
		if _, err := os.Stat(finalDir); err == nil {
			existing, err := s.readModelSnapshotUnlocked(snapshot.Manifest.SnapshotID)
			if err != nil {
				return err
			}
			if existing.Manifest.ContentHash != snapshot.Manifest.ContentHash {
				return fmt.Errorf("%w: model snapshot ID collision", ErrCorruptState)
			}
			snapshot = existing
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect model snapshot: %w", err)
		} else if err := s.writeModelSnapshotUnlocked(snapshot, finalDir); err != nil {
			return err
		}

		pointer := domain.ModelPointer{
			SchemaVersion:  domain.ModelSchemaVersion,
			ProjectID:      project.ProjectID,
			Repository:     project.Target,
			SnapshotID:     snapshot.Manifest.SnapshotID,
			ContentHash:    snapshot.Manifest.ContentHash,
			SourceRevision: snapshot.Manifest.SourceRevision,
			UpdatedAt:      snapshot.Manifest.GeneratedAt,
		}
		current, err := s.readModelPointerUnlocked()
		if err == nil && current.SnapshotID == pointer.SnapshotID && current.ContentHash == pointer.ContentHash {
			return nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := writeJSONAtomic(s.repository.Root, s.modelCurrentPath(), pointer); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return snapshot, changed, err
}

func (s Store) CurrentModel(ctx context.Context) (domain.ModelPointer, domain.ModelSnapshot, error) {
	var pointer domain.ModelPointer
	var snapshot domain.ModelSnapshot
	err := s.withLock(ctx, false, func() error {
		if err := s.verifyModelTree(); err != nil {
			return err
		}
		var err error
		pointer, err = s.readModelPointerUnlocked()
		if err != nil {
			return err
		}
		snapshot, err = s.readModelSnapshotUnlocked(pointer.SnapshotID)
		if err != nil {
			return err
		}
		if pointer.ContentHash != snapshot.Manifest.ContentHash ||
			pointer.SourceRevision != snapshot.Manifest.SourceRevision {
			return fmt.Errorf("%w: current model pointer does not match its snapshot", ErrCorruptState)
		}
		return nil
	})
	return pointer, snapshot, err
}

func (s Store) ModelSnapshot(ctx context.Context, snapshotID string) (domain.ModelSnapshot, error) {
	var snapshot domain.ModelSnapshot
	err := s.withLock(ctx, false, func() error {
		if err := s.verifyModelTree(); err != nil {
			return err
		}
		var err error
		snapshot, err = s.readModelSnapshotUnlocked(snapshotID)
		return err
	})
	return snapshot, err
}

func (s Store) modelRoot() string {
	return filepath.Join(s.stateDir(), "model")
}

func (s Store) modelCurrentPath() string {
	return filepath.Join(s.modelRoot(), "current.json")
}

func (s Store) modelSnapshotPath(snapshotID string) (string, error) {
	if !validModelSnapshotID(snapshotID) {
		return "", fmt.Errorf("%w: invalid model snapshot ID %q", ErrUnsafeStatePath, snapshotID)
	}
	return filepath.Join(s.modelRoot(), "snapshots", snapshotID), nil
}

func (s Store) writeModelSnapshotUnlocked(snapshot domain.ModelSnapshot, finalDir string) error {
	snapshotsDir := filepath.Dir(finalDir)
	if err := verifyContainedStatePath(s.repository.Root, snapshotsDir); err != nil {
		return err
	}
	if err := os.MkdirAll(snapshotsDir, 0o700); err != nil {
		return fmt.Errorf("create model snapshots directory: %w", err)
	}
	tempDir, err := os.MkdirTemp(snapshotsDir, ".snapshot-*")
	if err != nil {
		return fmt.Errorf("create model snapshot staging directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := writeJSONAtomic(s.repository.Root, filepath.Join(tempDir, "manifest.json"), snapshot.Manifest); err != nil {
		return err
	}
	if err := writeJSONAtomic(s.repository.Root, filepath.Join(tempDir, "vocabulary.json"), snapshot.Vocabulary); err != nil {
		return err
	}
	if err := writeModelJSONLAtomic(s.repository.Root, filepath.Join(tempDir, "nodes.jsonl"), snapshot.Nodes); err != nil {
		return err
	}
	if err := writeModelJSONLAtomic(s.repository.Root, filepath.Join(tempDir, "edges.jsonl"), snapshot.Edges); err != nil {
		return err
	}
	for name, content := range snapshot.Views {
		if err := writeModelFileAtomic(s.repository.Root, filepath.Join(tempDir, filepath.FromSlash(name)), []byte(content)); err != nil {
			return err
		}
	}
	if err := os.Rename(tempDir, finalDir); err != nil {
		return fmt.Errorf("publish model snapshot: %w", err)
	}
	parent, err := os.Open(snapshotsDir)
	if err != nil {
		return fmt.Errorf("open model snapshots directory for sync: %w", err)
	}
	defer parent.Close()
	if err := parent.Sync(); err != nil {
		return fmt.Errorf("sync model snapshots directory: %w", err)
	}
	return nil
}

func (s Store) readModelPointerUnlocked() (domain.ModelPointer, error) {
	var pointer domain.ModelPointer
	if err := readStrictJSON(s.modelCurrentPath(), &pointer); err != nil {
		return domain.ModelPointer{}, err
	}
	if err := pointer.Validate(); err != nil {
		return domain.ModelPointer{}, fmt.Errorf("%w: current repository model: %v", ErrCorruptState, err)
	}
	project, err := s.readProjectUnlocked()
	if err != nil {
		return domain.ModelPointer{}, err
	}
	if pointer.ProjectID != project.ProjectID || !pointer.Repository.Equal(project.Target) {
		return domain.ModelPointer{}, fmt.Errorf("%w: current repository model belongs to a different project", ErrIncompatibleState)
	}
	return pointer, nil
}

func (s Store) readModelSnapshotUnlocked(snapshotID string) (domain.ModelSnapshot, error) {
	dir, err := s.modelSnapshotPath(snapshotID)
	if err != nil {
		return domain.ModelSnapshot{}, err
	}
	var manifest domain.ModelManifest
	if err := readStrictJSON(filepath.Join(dir, "manifest.json"), &manifest); err != nil {
		return domain.ModelSnapshot{}, err
	}
	var vocabulary domain.ModelVocabulary
	if err := readStrictJSON(filepath.Join(dir, "vocabulary.json"), &vocabulary); err != nil {
		return domain.ModelSnapshot{}, err
	}
	nodes, err := readModelJSONL[domain.ModelNode](filepath.Join(dir, "nodes.jsonl"))
	if err != nil {
		return domain.ModelSnapshot{}, err
	}
	edges, err := readModelJSONL[domain.ModelEdge](filepath.Join(dir, "edges.jsonl"))
	if err != nil {
		return domain.ModelSnapshot{}, err
	}
	views, err := readModelViews(dir)
	if err != nil {
		return domain.ModelSnapshot{}, err
	}
	snapshot := domain.ModelSnapshot{
		Manifest: manifest, Vocabulary: vocabulary, Nodes: nodes, Edges: edges, Views: views,
	}
	if err := snapshot.Validate(); err != nil {
		return domain.ModelSnapshot{}, fmt.Errorf("%w: repository model snapshot %q: %v", ErrCorruptState, snapshotID, err)
	}
	project, err := s.readProjectUnlocked()
	if err != nil {
		return domain.ModelSnapshot{}, err
	}
	if manifest.ProjectID != project.ProjectID || !manifest.Repository.Equal(project.Target) {
		return domain.ModelSnapshot{}, fmt.Errorf("%w: repository model snapshot belongs to a different project", ErrIncompatibleState)
	}
	return snapshot, nil
}

func readStrictJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeModelJSONLAtomic[T any](repositoryRoot, destination string, values []T) error {
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			return fmt.Errorf("encode model JSONL: %w", err)
		}
	}
	return writeModelFileAtomic(repositoryRoot, destination, data.Bytes())
}

func readModelJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var values []T
	line := 0
	for scanner.Scan() {
		line++
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		var value T
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode %s line %d: %w", path, line, err)
		}
		var extra json.RawMessage
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			if err == nil {
				err = errors.New("multiple JSON values")
			}
			return nil, fmt.Errorf("decode %s line %d: %w", path, line, err)
		}
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return values, nil
}

func readModelViews(snapshotDir string) (map[string]string, error) {
	views := map[string]string{}
	for _, name := range []string{
		"index.md",
		"views/system-context.md",
		"views/domains.md",
		"views/components.md",
		"views/runtime.md",
		"views/data.md",
		"views/interfaces.md",
		"views/testing.md",
		"views/operations.md",
		"views/decisions.md",
		"views/vocabulary.md",
	} {
		data, err := os.ReadFile(filepath.Join(snapshotDir, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		views[name] = string(data)
	}
	return views, nil
}

func writeModelFileAtomic(repositoryRoot, destination string, data []byte) error {
	if err := verifyContainedStatePath(repositoryRoot, destination); err != nil {
		return err
	}
	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create model artifact parent: %w", err)
	}
	if err := verifyContainedStatePath(repositoryRoot, destination); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return fmt.Errorf("create atomic model artifact: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("set model artifact permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write model artifact: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync model artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close model artifact: %w", err)
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return fmt.Errorf("replace model artifact: %w", err)
	}
	parent, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open model artifact parent for sync: %w", err)
	}
	defer parent.Close()
	if err := parent.Sync(); err != nil {
		return fmt.Errorf("sync model artifact parent: %w", err)
	}
	return nil
}

func (s Store) verifyModelTree() error {
	root := s.modelRoot()
	if err := verifyContainedStatePath(s.repository.Root, root); err != nil {
		return err
	}
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect repository model: %w", err)
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink is not allowed: %s", ErrUnsafeStatePath, path)
		}
		return nil
	})
}

func validModelSnapshotID(value string) bool {
	if !strings.HasPrefix(value, "model-") || len(value) != 38 {
		return false
	}
	for _, char := range value[len("model-"):] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
