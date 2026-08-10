package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/testutil"
)

func TestModelSnapshotStorageIsAtomicAndIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := testutil.NewRepository(t)
	project, _, err := Initialize(ctx, repo.Root, stateTestTime)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := Load(ctx, repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	candidate := stateModelSnapshot(project, repo.Git("rev-parse", "HEAD"))

	written, changed, err := store.PutModelSnapshot(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first write was not reported as changed")
	}
	pointer, current, err := store.CurrentModel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pointer.ContentHash != written.Manifest.ContentHash ||
		current.Manifest.SnapshotID != written.Manifest.SnapshotID {
		t.Fatalf("current model does not match written model: %#v %#v", pointer, current.Manifest)
	}
	if _, err := os.Stat(filepath.Join(repo.Root, ".hermoso", "model", "snapshots", pointer.SnapshotID, "nodes.jsonl")); err != nil {
		t.Fatal(err)
	}

	repeated, changed, err := store.PutModelSnapshot(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if changed || repeated.Manifest.ContentHash != written.Manifest.ContentHash {
		t.Fatalf("repeat = changed %t, hash %s", changed, repeated.Manifest.ContentHash)
	}
}

func TestModelSnapshotStorageRejectsIdentityMismatchAndSymlinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := testutil.NewRepository(t)
	project, _, err := Initialize(ctx, repo.Root, stateTestTime)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := Load(ctx, repo.Root)
	if err != nil {
		t.Fatal(err)
	}

	wrong := stateModelSnapshot(project, repo.Git("rev-parse", "HEAD"))
	wrong.Manifest.ProjectID = "project-other"
	if _, _, err := store.PutModelSnapshot(ctx, wrong); !errors.Is(err, ErrIncompatibleState) {
		t.Fatalf("identity mismatch error = %v, want ErrIncompatibleState", err)
	}

	outside := t.TempDir()
	modelPath := filepath.Join(repo.Root, ".hermoso", "model")
	if err := os.Symlink(outside, modelPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.PutModelSnapshot(ctx, stateModelSnapshot(project, repo.Git("rev-parse", "HEAD"))); !errors.Is(err, ErrUnsafeStatePath) {
		t.Fatalf("symlink error = %v, want ErrUnsafeStatePath", err)
	}
}

func stateModelSnapshot(project domain.Project, revision string) domain.ModelSnapshot {
	readmeHash := digest.Bytes([]byte("# Test repository\n"))
	views := map[string]string{
		"index.md":                "# Repository\n",
		"views/system-context.md": "# System context\n",
		"views/domains.md":        "# Domains\n",
		"views/components.md":     "# Components\n",
		"views/runtime.md":        "# Runtime\n",
		"views/data.md":           "# Data\n",
		"views/interfaces.md":     "# Interfaces\n",
		"views/testing.md":        "# Testing\n",
		"views/operations.md":     "# Operations\n",
		"views/decisions.md":      "# Decisions\n",
		"views/vocabulary.md":     "# Vocabulary\n",
	}
	return domain.ModelSnapshot{
		Manifest: domain.ModelManifest{
			SchemaVersion: domain.ModelSchemaVersion, ProjectID: project.ProjectID,
			Repository: project.Target, SourceRevision: revision, VocabularyVersion: "v1",
			GeneratedAt: stateTestTime, Extractors: []domain.ModelExtractor{{Name: "git", Version: "1"}},
		},
		Vocabulary: domain.DefaultModelVocabulary(),
		Nodes: []domain.ModelNode{{
			ID: "file:README.md", Kind: "file", Abstraction: "code", Aspects: []string{"structure"},
			Title: "README.md", Summary: "Repository documentation.", EpistemicStatus: "observed",
			Evidence:   []domain.ModelEvidence{{Path: "README.md", ContentHash: readmeHash}},
			Producer:   domain.ModelProducer{Kind: "extractor", Name: "git", Version: "1"},
			Attributes: map[string]string{"language": "Markdown"},
		}},
		Views: views,
	}
}

func TestModelSnapshotRejectsCorruptJSONL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := testutil.NewRepository(t)
	project, _, err := Initialize(ctx, repo.Root, stateTestTime)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := Load(ctx, repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	written, _, err := store.PutModelSnapshot(ctx, stateModelSnapshot(project, repo.Git("rev-parse", "HEAD")))
	if err != nil {
		t.Fatal(err)
	}
	nodesPath := filepath.Join(repo.Root, ".hermoso", "model", "snapshots", written.Manifest.SnapshotID, "nodes.jsonl")
	data, err := os.ReadFile(nodesPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodesPath, append(data, []byte(`{"surprise":true}`+"\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CurrentModel(ctx); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field", err)
	}
}
