package testutil

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/domain"
)

func ModelSnapshot(t *testing.T, context domain.ContextRef) domain.ModelSnapshot {
	t.Helper()
	command := exec.Command("git", "-C", context.Repository.Repository, "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	sourceRevision := strings.TrimSpace(string(output))
	views := map[string]string{}
	for _, path := range []string{
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
		views[path] = "# Test model\n"
	}
	snapshot, err := domain.FinalizeModelSnapshot(domain.ModelSnapshot{
		Manifest: domain.ModelManifest{
			SchemaVersion:     domain.ModelSchemaVersion,
			ProjectID:         context.ProjectID,
			Repository:        context.Repository,
			SourceRevision:    sourceRevision,
			VocabularyVersion: "v1",
			GeneratedAt:       time.Date(2026, 8, 2, 15, 0, 0, 0, time.UTC),
			Extractors:        []domain.ModelExtractor{{Name: "test", Version: "1"}},
		},
		Vocabulary: domain.DefaultModelVocabulary(),
		Nodes: []domain.ModelNode{{
			ID: "repository:test", Kind: "repository", Abstraction: "system",
			Aspects: []string{"structure"}, Title: "Test repository", Summary: "Test repository.",
			EpistemicStatus: "observed",
			Evidence: []domain.ModelEvidence{{
				Path: "README.md", ContentHash: digest.Bytes([]byte("test repository")),
			}},
			Producer: domain.ModelProducer{Kind: "extractor", Name: "test", Version: "1"},
		}},
		Views: views,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func ModelReference(snapshot domain.ModelSnapshot) domain.ModelReference {
	return domain.ModelReference{
		SchemaVersion: snapshot.Manifest.SchemaVersion,
		SnapshotID:    snapshot.Manifest.SnapshotID, ContentHash: snapshot.Manifest.ContentHash,
		SourceRevision:    snapshot.Manifest.SourceRevision,
		VocabularyVersion: snapshot.Manifest.VocabularyVersion,
	}
}
