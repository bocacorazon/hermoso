package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/bocacorazon/hermoso/internal/digest"
)

func TestModelSnapshotFinalizationIsDeterministic(t *testing.T) {
	t.Parallel()
	first := validModelSnapshot()
	second := validModelSnapshot()
	second.Manifest.GeneratedAt = second.Manifest.GeneratedAt.Add(time.Hour)
	second.Nodes[0].Aspects = []string{"runtime", "structure"}

	first, err := FinalizeModelSnapshot(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err = FinalizeModelSnapshot(second)
	if err != nil {
		t.Fatal(err)
	}
	if first.Manifest.ContentHash != second.Manifest.ContentHash ||
		first.Manifest.SnapshotID != second.Manifest.SnapshotID {
		t.Fatalf("equivalent snapshots differ:\n%#v\n%#v", first.Manifest, second.Manifest)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("finalized snapshot does not validate: %v", err)
	}
}

func TestModelSnapshotRejectsBrokenTraceability(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(*ModelSnapshot)
		want string
	}{
		{
			name: "dangling edge",
			edit: func(snapshot *ModelSnapshot) { snapshot.Edges[0].Target = "file:missing.go" },
			want: "references an unknown node",
		},
		{
			name: "unsafe evidence path",
			edit: func(snapshot *ModelSnapshot) { snapshot.Nodes[0].Evidence[0].Path = "../outside" },
			want: "clean repository-relative path",
		},
		{
			name: "unknown interface kind",
			edit: func(snapshot *ModelSnapshot) {
				snapshot.Nodes[1].Attributes["interface_kind"] = "telepathy"
			},
			want: "versioned model vocabulary",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			snapshot := validModelSnapshot()
			test.edit(&snapshot)
			if _, err := FinalizeModelSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func validModelSnapshot() ModelSnapshot {
	readmeHash := digest.Bytes([]byte("# repository\n"))
	producer := ModelProducer{Kind: "extractor", Name: "git", Version: "1"}
	views := map[string]string{}
	for _, name := range requiredModelViews() {
		views[name] = "# " + name + "\n"
	}
	return ModelSnapshot{
		Manifest: ModelManifest{
			SchemaVersion:     ModelSchemaVersion,
			ProjectID:         "project-test",
			Repository:        TargetIdentity{Repository: "/tmp/repository", DefaultBranch: "main"},
			SourceRevision:    strings.Repeat("a", 40),
			VocabularyVersion: "v1",
			GeneratedAt:       time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
			Extractors:        []ModelExtractor{{Name: "git", Version: "1"}},
		},
		Vocabulary: DefaultModelVocabulary(),
		Nodes: []ModelNode{
			{
				ID: "file:README.md", Kind: "file", Abstraction: "code",
				Aspects: []string{"structure", "runtime"}, Title: "README.md",
				Summary: "Repository documentation.", EpistemicStatus: "observed",
				Evidence: []ModelEvidence{{Path: "README.md", ContentHash: readmeHash}},
				Producer: producer,
			},
			{
				ID: "interface:cli", Kind: "interface", Abstraction: "system",
				Aspects: []string{"api"}, Title: "CLI", Summary: "Command-line interface.",
				EpistemicStatus: "derived", DerivedFrom: []string{"file:README.md"},
				Producer:   ModelProducer{Kind: "algorithm", Name: "interface-classifier", Version: "1"},
				Attributes: map[string]string{"interface_kind": "cli"},
			},
		},
		Edges: []ModelEdge{{
			ID: "edge:cli-source", Source: "interface:cli", Relation: "derived_from",
			Target: "file:README.md", EpistemicStatus: "derived",
			Evidence: []string{"file:README.md"},
			Producer: ModelProducer{Kind: "algorithm", Name: "interface-classifier", Version: "1"},
		}},
		Views: views,
	}
}
