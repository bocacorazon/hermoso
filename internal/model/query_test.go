package model

import (
	"context"
	"strings"
	"testing"
)

func TestQueryProvidesBoundedTaskImpactAndEvidenceViews(t *testing.T) {
	t.Parallel()
	repo, project := modelRepository(t)
	revision := repo.Commit("add model fixtures")
	snapshot, err := Build(context.Background(), BuildRequest{
		Project: project, RepositoryRoot: repo.Root, Revision: revision, GeneratedAt: modelTestTime,
	})
	if err != nil {
		t.Fatal(err)
	}

	task, err := Query(snapshot, QueryRequest{Mode: QueryTask, Text: "handler api", BudgetBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Nodes) == 0 || !strings.Contains(task.Rendered, "Handler") {
		t.Fatalf("task query did not return Handler: %#v", task)
	}

	var interfaceID string
	for _, node := range snapshot.Nodes {
		if node.Kind == "interface" && node.Title == "Handler" {
			interfaceID = node.ID
			break
		}
	}
	if interfaceID == "" {
		t.Fatal("fixture has no Handler interface")
	}
	impact, err := Query(snapshot, QueryRequest{Mode: QueryImpact, NodeIDs: []string{interfaceID}, BudgetBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if len(impact.Nodes) < 2 || len(impact.Edges) == 0 {
		t.Fatalf("impact query did not traverse relationships: %#v", impact)
	}
	evidence, err := Query(snapshot, QueryRequest{Mode: QueryEvidence, NodeIDs: []string{interfaceID}, BudgetBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Nodes) != 1 || evidence.Nodes[0].ID != interfaceID {
		t.Fatalf("evidence query = %#v", evidence)
	}
}

func TestQueryRejectsUnknownNodesAndUnboundedMinimums(t *testing.T) {
	t.Parallel()
	repo, project := modelRepository(t)
	revision := repo.Commit("add model fixtures")
	snapshot, err := Build(context.Background(), BuildRequest{
		Project: project, RepositoryRoot: repo.Root, Revision: revision, GeneratedAt: modelTestTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Query(snapshot, QueryRequest{Mode: QueryImpact, NodeIDs: []string{"missing"}, BudgetBytes: 4096}); err == nil {
		t.Fatal("unknown impact node was accepted")
	}
	if _, err := Query(snapshot, QueryRequest{Mode: QueryOrientation, BudgetBytes: 100}); err == nil {
		t.Fatal("undersized query budget was accepted")
	}
}
