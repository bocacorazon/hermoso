package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"

	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/state"
	"github.com/bocacorazon/hermoso/internal/testutil"
)

var modelTestTime = time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)

func TestBuildIndexesSupportedLanguagesSurfacesAndCommands(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, project := modelRepository(t)
	revision := repo.Commit("add model fixtures")

	snapshot, err := Build(ctx, BuildRequest{
		Project: project, RepositoryRoot: repo.Root, Revision: revision, GeneratedAt: modelTestTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"Go", "JavaScript", "TypeScript", "TSX", "Python"} {
		if !hasNodeAttribute(snapshot.Nodes, "language", language) {
			t.Errorf("snapshot has no %s node", language)
		}
	}
	for _, kind := range []string{"repository", "file", "symbol", "component", "interface", "test", "command", "concept", "invariant"} {
		if !hasNodeKind(snapshot.Nodes, kind) {
			t.Errorf("snapshot has no %s node", kind)
		}
	}
	if !hasEdgeRelation(snapshot.Edges, "imports") {
		t.Error("snapshot has no import edge")
	}
	if !hasEdgeRelation(snapshot.Edges, "exercises") {
		t.Error("snapshot has no test exercise edge")
	}
	if !strings.Contains(snapshot.Views["views/interfaces.md"], "interface:") ||
		!strings.Contains(snapshot.Views["views/testing.md"], "Exercises") {
		t.Fatalf("rendered views lack indexed data: %#v", snapshot.Views)
	}

	repeated, err := Build(ctx, BuildRequest{
		Project: project, RepositoryRoot: repo.Root, Revision: revision,
		GeneratedAt: modelTestTime.Add(time.Hour), Previous: &snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Manifest.ContentHash != snapshot.Manifest.ContentHash {
		t.Errorf("incremental equivalent hash = %s, want %s", repeated.Manifest.ContentHash, snapshot.Manifest.ContentHash)
	}
}

func TestBuildInvalidatesChangedEvidenceWithoutStaleSymbols(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, project := modelRepository(t)
	firstRevision := repo.Commit("add model fixtures")
	first, err := Build(ctx, BuildRequest{
		Project: project, RepositoryRoot: repo.Root, Revision: firstRevision, GeneratedAt: modelTestTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	repo.Write("src/app.ts", "export interface Replacement { id: string }\n")
	secondRevision := repo.Commit("replace TypeScript declaration")
	second, err := Build(ctx, BuildRequest{
		Project: project, RepositoryRoot: repo.Root, Revision: secondRevision,
		GeneratedAt: modelTestTime.Add(time.Minute), Previous: &first,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Manifest.ContentHash == first.Manifest.ContentHash {
		t.Fatal("changed source produced the same snapshot hash")
	}
	if hasNodeTitle(second.Nodes, "App") {
		t.Error("incremental model retained removed TypeScript symbol")
	}
	if !hasNodeTitle(second.Nodes, "Replacement") {
		t.Error("incremental model omitted replacement TypeScript symbol")
	}
}

func TestBuildConsumesExplicitSCIPAndRejectsMalformedIndexes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, project := modelRepository(t)
	revision := repo.Commit("add model fixtures")
	index := &scip.Index{
		Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "scip-test", Version: "1.0"}},
		Documents: []*scip.Document{{
			Language: "go", RelativePath: "main.go",
			Occurrences: []*scip.Occurrence{{
				Range: []int32{4, 5, 9}, Symbol: "scip . . . Main().",
				SymbolRoles: int32(scip.SymbolRole_Definition),
			}},
		}},
	}
	data, err := proto.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(indexPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Build(ctx, BuildRequest{
		Project: project, RepositoryRoot: repo.Root, Revision: revision,
		GeneratedAt: modelTestTime, SCIPPath: indexPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasNodePrefix(snapshot.Nodes, "symbol:scip:") {
		t.Error("SCIP symbol was not indexed")
	}

	badPath := filepath.Join(t.TempDir(), "bad.scip")
	if err := os.WriteFile(badPath, []byte("not protobuf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(ctx, BuildRequest{
		Project: project, RepositoryRoot: repo.Root, Revision: revision,
		GeneratedAt: modelTestTime, SCIPPath: badPath,
	}); err == nil || !strings.Contains(err.Error(), "decode SCIP") {
		t.Fatalf("malformed SCIP error = %v", err)
	}
}

func modelRepository(t *testing.T) (*testutil.Repository, domain.Project) {
	t.Helper()
	repo := testutil.NewRepository(t)
	repo.Write("go.mod", "module example.com/modeltest\n\ngo 1.26\n")
	repo.Write("main.go", `package main

import "net/http"

func Main() {}
func Handler(w http.ResponseWriter, r *http.Request) {}
`)
	repo.Write("main_test.go", `package main

import "testing"

func TestMainFunction(t *testing.T) { Main() }
`)
	repo.Write("src/helper.js", "export function helper() { return true }\n")
	repo.Write("src/app.ts", "import { helper } from './helper.js'\nexport interface App { id: string }\n")
	repo.Write("src/view.tsx", "export function Card() { return <div>card</div> }\n")
	repo.Write("python/module.py", "import json\n\ndef answer():\n    return 42\n")
	repo.Write("python/test_module.py", "from module import answer\n\ndef test_answer():\n    assert answer() == 42\n")
	repo.Write("package.json", `{"scripts":{"test":"node --test","build":"tsc"},"bin":{"model":"cli.js"}}`)
	repo.Write("docs/domain.md", "# Model Domain\n\n## Invariants\n")
	project, _, err := state.Initialize(context.Background(), repo.Root, modelTestTime)
	if err != nil {
		t.Fatal(err)
	}
	return repo, project
}

func hasNodeKind(nodes []domain.ModelNode, kind string) bool {
	for _, node := range nodes {
		if node.Kind == kind {
			return true
		}
	}
	return false
}

func hasNodeAttribute(nodes []domain.ModelNode, key, value string) bool {
	for _, node := range nodes {
		if node.Attributes[key] == value {
			return true
		}
	}
	return false
}

func hasNodeTitle(nodes []domain.ModelNode, title string) bool {
	for _, node := range nodes {
		if node.Title == title {
			return true
		}
	}
	return false
}

func hasNodePrefix(nodes []domain.ModelNode, prefix string) bool {
	for _, node := range nodes {
		if strings.HasPrefix(node.ID, prefix) {
			return true
		}
	}
	return false
}

func hasEdgeRelation(edges []domain.ModelEdge, relation string) bool {
	for _, edge := range edges {
		if edge.Relation == relation {
			return true
		}
	}
	return false
}
