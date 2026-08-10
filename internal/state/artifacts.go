package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/domain"
)

const sealedArtifactsSchemaVersion = "hermoso-sealed-artifacts/v1"

type sealedArtifactEntry struct {
	Path        string `json:"path"`
	ContentHash string `json:"content_hash"`
}

type sealedArtifactManifest struct {
	SchemaVersion string                `json:"schema_version"`
	Context       domain.ContextRef     `json:"context"`
	RootHash      string                `json:"root_hash"`
	Entries       []sealedArtifactEntry `json:"entries"`
}

func (s Store) SealArtifacts(ctx context.Context, execution domain.ContextRef, assets map[string][]byte) (string, error) {
	if len(assets) == 0 {
		return "", errors.New("sealed artifact set must not be empty")
	}
	entries := make([]sealedArtifactEntry, 0, len(assets))
	for assetPath, data := range assets {
		if !safeArtifactPath(assetPath) {
			return "", fmt.Errorf("%w: unsafe artifact path %q", ErrUnsafeStatePath, assetPath)
		}
		entries = append(entries, sealedArtifactEntry{Path: assetPath, ContentHash: digest.Bytes(data)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	rootHash, err := digest.JSON(entries)
	if err != nil {
		return "", fmt.Errorf("hash sealed artifact set: %w", err)
	}
	manifest := sealedArtifactManifest{
		SchemaVersion: sealedArtifactsSchemaVersion, Context: execution, RootHash: rootHash, Entries: entries,
	}

	err = s.withLock(ctx, true, func() error {
		project, err := s.readProjectUnlocked()
		if err != nil {
			return err
		}
		var run domain.Run
		if err := readStateJSON(s.runPath(execution.RunID), "run", &run); err != nil {
			return err
		}
		if err := validateRunIdentity(project, execution.RunID, run); err != nil {
			return err
		}
		if !run.Context.Equal(execution) {
			return fmt.Errorf("%w: sealed artifact context does not match persisted run", ErrIncompatibleState)
		}
		finalDir := s.sealedArtifactPath(execution.RunID, rootHash)
		if err := s.verifyArtifactTree(); err != nil {
			return err
		}
		if _, err := os.Stat(finalDir); err == nil {
			_, existing, err := s.readSealedArtifactsUnlocked(execution, rootHash)
			if err != nil {
				return err
			}
			for assetPath, data := range assets {
				if digest.Bytes(existing[assetPath]) != digest.Bytes(data) {
					return fmt.Errorf("%w: sealed artifact root collision", ErrCorruptState)
				}
			}
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect sealed artifacts: %w", err)
		}

		parent := filepath.Dir(finalDir)
		if err := verifyContainedStatePath(s.repository.Root, parent); err != nil {
			return err
		}
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return fmt.Errorf("create sealed artifact store: %w", err)
		}
		tempDir, err := os.MkdirTemp(parent, ".artifacts-*")
		if err != nil {
			return fmt.Errorf("create sealed artifact staging directory: %w", err)
		}
		defer os.RemoveAll(tempDir)
		if err := writeJSONAtomic(s.repository.Root, filepath.Join(tempDir, "manifest.json"), manifest); err != nil {
			return err
		}
		for assetPath, data := range assets {
			destination := filepath.Join(tempDir, "files", filepath.FromSlash(assetPath))
			if err := writeModelFileAtomic(s.repository.Root, destination, data); err != nil {
				return err
			}
		}
		if err := os.Rename(tempDir, finalDir); err != nil {
			return fmt.Errorf("publish sealed artifacts: %w", err)
		}
		directory, err := os.Open(parent)
		if err != nil {
			return fmt.Errorf("open artifact store for sync: %w", err)
		}
		defer directory.Close()
		return directory.Sync()
	})
	return rootHash, err
}

func (s Store) ReadSealedArtifacts(ctx context.Context, execution domain.ContextRef, rootHash string) (map[string][]byte, error) {
	var assets map[string][]byte
	err := s.withLock(ctx, false, func() error {
		project, err := s.readProjectUnlocked()
		if err != nil {
			return err
		}
		var run domain.Run
		if err := readStateJSON(s.runPath(execution.RunID), "run", &run); err != nil {
			return err
		}
		if err := validateRunIdentity(project, execution.RunID, run); err != nil {
			return err
		}
		if !run.Context.Equal(execution) {
			return fmt.Errorf("%w: sealed artifact context does not match persisted run", ErrIncompatibleState)
		}
		_, assets, err = s.readSealedArtifactsUnlocked(execution, rootHash)
		return err
	})
	return assets, err
}

func (s Store) MaterializeVerificationArtifacts(
	ctx context.Context,
	execution domain.ContextRef,
	attempt uint64,
	rootHash string,
	artifacts []domain.VerificationArtifact,
) (string, error) {
	if attempt == 0 || attempt > 2 {
		return "", errors.New("verification attempt must be one or two")
	}
	assets, err := s.ReadSealedArtifacts(ctx, execution, rootHash)
	if err != nil {
		return "", err
	}
	root := filepath.Join(
		s.stateDir(), "verification", execution.RunID,
		fmt.Sprintf("attempt-%d", attempt), "assets",
	)
	if err := verifyContainedStatePath(s.repository.Root, root); err != nil {
		return "", err
	}
	if err := os.RemoveAll(root); err != nil {
		return "", fmt.Errorf("reset verification asset directory: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create verification asset directory: %w", err)
	}
	for _, artifact := range artifacts {
		data, ok := assets[artifact.Path]
		if !ok {
			return "", fmt.Errorf("%w: sealed verification artifact %q is missing", ErrCorruptState, artifact.Path)
		}
		destination := filepath.Join(root, filepath.FromSlash(artifact.Path))
		if err := verifyContainedStatePath(s.repository.Root, destination); err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return "", fmt.Errorf("create verification artifact parent: %w", err)
		}
		mode := os.FileMode(0o400)
		if artifact.Kind == "probe" || artifact.Kind == "generated_test" {
			mode = 0o500
		}
		if err := os.WriteFile(destination, data, mode); err != nil {
			return "", fmt.Errorf("materialize verification artifact %q: %w", artifact.ID, err)
		}
	}
	return root, nil
}

func (s Store) readSealedArtifactsUnlocked(execution domain.ContextRef, rootHash string) (sealedArtifactManifest, map[string][]byte, error) {
	if !hashPatternString(rootHash) {
		return sealedArtifactManifest{}, nil, fmt.Errorf("%w: invalid sealed artifact root hash", ErrUnsafeStatePath)
	}
	dir := s.sealedArtifactPath(execution.RunID, rootHash)
	var manifest sealedArtifactManifest
	if err := readStrictJSON(filepath.Join(dir, "manifest.json"), &manifest); err != nil {
		return sealedArtifactManifest{}, nil, err
	}
	if manifest.SchemaVersion != sealedArtifactsSchemaVersion || manifest.RootHash != rootHash {
		return sealedArtifactManifest{}, nil, fmt.Errorf("%w: sealed artifact manifest identity is invalid", ErrCorruptState)
	}
	if err := manifest.Context.Validate(); err != nil {
		return sealedArtifactManifest{}, nil, fmt.Errorf("%w: sealed artifact context: %v", ErrCorruptState, err)
	}
	if !manifest.Context.Equal(execution) {
		return sealedArtifactManifest{}, nil, fmt.Errorf("%w: sealed artifacts belong to a different run", ErrIncompatibleState)
	}
	entriesHash, err := digest.JSON(manifest.Entries)
	if err != nil || entriesHash != rootHash {
		return sealedArtifactManifest{}, nil, fmt.Errorf("%w: sealed artifact root hash does not match manifest", ErrCorruptState)
	}
	assets := make(map[string][]byte, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if !safeArtifactPath(entry.Path) || !hashPatternString(entry.ContentHash) {
			return sealedArtifactManifest{}, nil, fmt.Errorf("%w: invalid sealed artifact entry", ErrCorruptState)
		}
		data, err := os.ReadFile(filepath.Join(dir, "files", filepath.FromSlash(entry.Path)))
		if err != nil {
			return sealedArtifactManifest{}, nil, err
		}
		if digest.Bytes(data) != entry.ContentHash {
			return sealedArtifactManifest{}, nil, fmt.Errorf("%w: sealed artifact %q content hash changed", ErrCorruptState, entry.Path)
		}
		assets[entry.Path] = data
	}
	return manifest, assets, nil
}

func (s Store) sealedArtifactPath(runID, rootHash string) string {
	return filepath.Join(s.stateDir(), "artifacts", runID, strings.TrimPrefix(rootHash, "sha256:"))
}

func (s Store) verifyArtifactTree() error {
	root := filepath.Join(s.stateDir(), "artifacts")
	if err := verifyContainedStatePath(s.repository.Root, root); err != nil {
		return err
	}
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
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

func safeArtifactPath(value string) bool {
	return value != "" && value == filepath.ToSlash(filepath.Clean(value)) &&
		!filepath.IsAbs(value) && value != "." && value != ".." &&
		!strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\x00\r\n\\")
}

func hashPatternString(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
