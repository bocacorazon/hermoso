package dispatch

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func TestCompileTopologyParallelRootsAndFanIn(t *testing.T) {
	graph := testGraph([]domain.WorkItem{
		item("join", []string{"right", "left"}),
		item("right", nil),
		item("left", nil),
	})
	plan, err := Compile(graph, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cardIDs(plan.Cards), []string{"left", "right", "join"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("card order = %v, want %v", got, want)
	}
	if got, want := parentIDs(plan.Cards[2]), []string{"left", "right"}; !reflect.DeepEqual(got, want) {
		t.Errorf("join parents = %v, want %v", got, want)
	}
	reordered := testGraph([]domain.WorkItem{
		item("left", nil),
		item("join", []string{"left", "right"}),
		item("right", nil),
	})
	recompiled, err := Compile(reordered, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(plan)
	b, _ := json.Marshal(recompiled)
	if string(a) != string(b) {
		t.Fatalf("semantic recompilation differs:\n%s\n%s", a, b)
	}
	ready, err := plan.CreateReadyCards(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cardIDs(ready), []string{"left", "right"}; !reflect.DeepEqual(got, want) {
		t.Errorf("initial ready cards = %v, want %v", got, want)
	}
	bindings := []domain.TaskBinding{
		{Context: graph.Context, WorkItemID: "left", TaskID: "task-left", BoundAt: time.Now()},
		{Context: graph.Context, WorkItemID: "right", TaskID: "task-right", BoundAt: time.Now()},
	}
	ready, err = plan.CreateReadyCards(bindings)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cardIDs(ready), []string{"join"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ready cards = %v, want %v", got, want)
	}
	if got := []string{ready[0].Parents[0].TaskID, ready[0].Parents[1].TaskID}; !reflect.DeepEqual(got, []string{"task-left", "task-right"}) {
		t.Errorf("resolved parent tasks = %v", got)
	}
}

func TestResolveParentsReportsMissingBinding(t *testing.T) {
	plan, err := Compile(testGraph([]domain.WorkItem{item("root", nil), item("child", []string{"root"})}), testConfig())
	if err != nil {
		t.Fatal(err)
	}
	_, err = ResolveParents(plan.Cards[1], nil)
	if !errors.Is(err, ErrMissingParentBinding) {
		t.Fatalf("error = %v, want ErrMissingParentBinding", err)
	}
}

func TestReadyRejectsBindingFromAnotherContext(t *testing.T) {
	plan, err := Compile(testGraph([]domain.WorkItem{item("root", nil)}), testConfig())
	if err != nil {
		t.Fatal(err)
	}
	other := plan.Context
	other.FeatureID = "other-feature"
	_, err = plan.Ready([]domain.TaskBinding{{
		Context: other, WorkItemID: "root", TaskID: "task-root", BoundAt: time.Now(),
	}})
	if err == nil || !strings.Contains(err.Error(), "context does not match") {
		t.Fatalf("mismatch error = %v", err)
	}
}

func TestCompileIsStableAndCarriesRuntimeContract(t *testing.T) {
	graph := testGraph([]domain.WorkItem{item("root", nil)})
	graph.Items[0].ValidationCommands = []string{"go test ./internal/root"}
	first, err := Compile(graph, testConfig())
	if err != nil {
		t.Fatal(err)
	}

	second, err := Compile(graph, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatalf("repeated compilation differs:\n%s\n%s", a, b)
	}
	otherGraph := graph
	otherGraph.Context.FeatureID = "feature-2"
	other, err := Compile(otherGraph, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if first.Cards[0].IdempotencyKey == other.Cards[0].IdempotencyKey {
		t.Fatal("idempotency key does not include full execution context")
	}
	card := first.Cards[0]
	if card.WorkspaceKind != "worktree" || card.WorkspacePath != "/workspaces/run-1/items/root" {
		t.Errorf("workspace = %q %q", card.WorkspaceKind, card.WorkspacePath)
	}
	if card.Tenant != "hermoso-project-1" || card.Priority != 7 || card.AssignedProfile != "builder" {
		t.Errorf("dispatch assignment not preserved: %#v", card)
	}
	if got := card.ForcedSkills[0].Name; got != "tdd" {
		t.Errorf("forced skill = %q", got)
	}
	if card.RuntimeBudgetSeconds != 900 || card.IdempotencyKey == "" {
		t.Errorf("runtime/idempotency missing: %#v", card)
	}
	if got, want := card.LifecycleCommands.Validate, []string{
		`test "$PWD" = '/workspaces/run-1/items/root' && hermoso context 'project-1' 'feature-1' 'run-1' '/workspace/project' --json`,
		"go vet ./...", "go test ./internal/root",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("validation lifecycle = %v, want %v", got, want)
	}

	if !card.Identity.Context.Equal(graph.Context) || !strings.Contains(card.Body, `"project_id":"project-1"`) ||
		!strings.Contains(card.Body, "Absolute workspace: /workspaces/run-1/items/root") {
		t.Errorf("card does not repeat execution context and workspace: %#v", card)
	}
}

func TestLifecycleContextCommandPOSIXQuotesEveryDynamicArgument(t *testing.T) {
	context := testContext()
	context.ProjectID = "project'a $() `tick`"
	context.FeatureID = "feature'a $() `tick`"
	context.RunID = "run'a $() `tick`"
	context.Repository.Repository = "/workspace/a'post $() `tick`"
	command, err := lifecycleContextCommand("/work trees/a'post $() `tick`", context)
	if err != nil {
		t.Fatal(err)
	}
	want := `test "$PWD" = '/work trees/a'"'"'post $() ` + "`tick`" +
		`' && hermoso context 'project'"'"'a $() ` + "`tick`" +
		`' 'feature'"'"'a $() ` + "`tick`" +
		`' 'run'"'"'a $() ` + "`tick`" +
		`' '/workspace/a'"'"'post $() ` + "`tick`" + `' --json`
	if command != want {
		t.Fatalf("command = %q, want %q", command, want)
	}

	tests := []struct {
		name      string
		workspace string
		change    func(*domain.ContextRef)
	}{
		{"workspace", "/workspace/line\nbreak", func(*domain.ContextRef) {}},
		{"project", "/workspace/ok", func(c *domain.ContextRef) { c.ProjectID = "line\nbreak" }},
		{"feature", "/workspace/ok", func(c *domain.ContextRef) { c.FeatureID = "line\nbreak" }},
		{"run", "/workspace/ok", func(c *domain.ContextRef) { c.RunID = "line\nbreak" }},
		{"repository", "/workspace/ok", func(c *domain.ContextRef) { c.Repository.Repository = "line\nbreak" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := testContext()
			test.change(&candidate)
			if _, err := lifecycleContextCommand(test.workspace, candidate); err == nil || !strings.Contains(err.Error(), "newlines") {
				t.Fatalf("newline error = %v", err)
			}
		})
	}
}

func TestCardIdentityAndIdempotencyIsolateEveryContextAxis(t *testing.T) {
	baseGraph := testGraph([]domain.WorkItem{item("root", nil)})
	base, err := Compile(baseGraph, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	baseCard := base.Cards[0]

	tests := []struct {
		name   string
		change func(*domain.ContextRef)
	}{
		{"project", func(context *domain.ContextRef) { context.ProjectID = "project-2" }},
		{"feature", func(context *domain.ContextRef) { context.FeatureID = "feature-2" }},
		{"run", func(context *domain.ContextRef) { context.RunID = "run-2" }},
		{"repository", func(context *domain.ContextRef) { context.Repository.Repository = "/workspace/other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := baseGraph
			test.change(&graph.Context)
			plan, err := Compile(graph, testConfig())
			if err != nil {
				t.Fatal(err)
			}
			card := plan.Cards[0]
			if card.IdempotencyKey == baseCard.IdempotencyKey {
				t.Fatalf("%s identity did not affect idempotency key", test.name)
			}
			if !card.Identity.Context.Equal(graph.Context) {
				t.Fatalf("%s identity was not preserved in card", test.name)
			}
			if test.name == "project" && card.Tenant == baseCard.Tenant {
				t.Fatal("project identity did not isolate Kanban tenant")
			}
		})
	}
}

func TestSyntheticIntegrationDependsOnAllLeaves(t *testing.T) {
	config := testConfig()
	config.Integration = &SyntheticIntegration{
		ID: "integrate", Title: "Integrate", Body: "Integrate all branches",
		AcceptanceCriteria: []string{"Combined validation passes"},
		Worker:             domain.Worker{Profile: "integrator", Skills: []domain.SkillBinding{{Name: "integration"}}},
		GoalMode:           "merge-and-verify",
	}
	graph := testGraph([]domain.WorkItem{
		item("root", nil),
		item("leaf-a", []string{"root"}),
		item("leaf-b", []string{"root"}),
	})
	plan, err := Compile(graph, config)
	if err != nil {
		t.Fatal(err)
	}
	card := plan.Cards[len(plan.Cards)-1]
	if !card.Identity.Synthetic || card.GoalMode != "merge-and-verify" {
		t.Errorf("integration contract = %#v", card)
	}
	if got, want := parentIDs(card), []string{"leaf-a", "leaf-b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("integration parents = %v, want %v", got, want)
	}
}

func TestCompileRejectsInvalidPathsAndConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"relative path", func(c *Config) { c.PreparedWorkspaces["root"] = "workspaces" }},
		{"unclean path", func(c *Config) { c.PreparedWorkspaces["root"] = "/workspaces/.." }},
		{"missing path", func(c *Config) { delete(c.PreparedWorkspaces, "root") }},
		{"negative priority", func(c *Config) { c.Priority = -1 }},
		{"zero budget", func(c *Config) { c.DefaultRuntimeBudgetSeconds = 0 }},
		{"blank command", func(c *Config) { c.LifecycleCommands.Execute = []string{""} }},
		{"shared path", func(c *Config) {
			c.PreparedWorkspaces["child"] = c.PreparedWorkspaces["root"]
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := testConfig()
			test.change(&config)
			items := []domain.WorkItem{item("root", nil)}
			if test.name == "shared path" {
				items = append(items, item("child", []string{"root"}))
			}
			if _, err := Compile(testGraph(items), config); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func testGraph(items []domain.WorkItem) domain.WorkGraph {
	return domain.WorkGraph{
		SchemaVersion: domain.SchemaVersion, Context: testContext(), Revision: 1,
		Producer: domain.Producer{Skill: "construction", Runtime: "hermes"},
		Items:    items,
	}
}

func item(id string, parents []string) domain.WorkItem {
	return domain.WorkItem{
		ID: id, Title: "Build " + id, Prompt: "Implement " + id,
		Parents: parents, AcceptanceCriteria: []string{id + " works"},
		Worker: domain.Worker{Profile: "builder", Skills: []domain.SkillBinding{{Name: "tdd"}, {Name: "review"}}},
	}
}

func testConfig() Config {
	return Config{
		Priority: 7,
		PreparedWorkspaces: map[string]string{
			"root":      "/workspaces/run-1/items/root",
			"left":      "/workspaces/run-1/items/left",
			"right":     "/workspaces/run-1/items/right",
			"join":      "/workspaces/run-1/items/join",
			"child":     "/workspaces/run-1/items/child",
			"leaf-a":    "/workspaces/run-1/items/leaf-a",
			"leaf-b":    "/workspaces/run-1/items/leaf-b",
			"integrate": "/workspaces/run-1/integration/integrate",
		},
		DefaultRuntimeBudgetSeconds: 900,
		LifecycleCommands: LifecycleCommands{
			Prepare: []string{"git status"}, Execute: []string{"go test ./..."}, Validate: []string{"go vet ./..."},
		},
	}

}

func testContext() domain.ContextRef {
	return domain.ContextRef{
		SchemaVersion: domain.ContextSchemaVersion,
		ProjectID:     "project-1",
		FeatureID:     "feature-1",
		RunID:         "run-1",
		Repository:    domain.TargetIdentity{Repository: "/workspace/project"},
	}
}

func cardIDs(cards []Card) []string {
	ids := make([]string, len(cards))
	for i, card := range cards {
		ids[i] = card.Identity.WorkItemID
	}
	return ids
}

func parentIDs(card Card) []string {
	ids := make([]string, len(card.Parents))
	for i, parent := range card.Parents {
		ids[i] = parent.WorkItemID
	}
	return ids
}
