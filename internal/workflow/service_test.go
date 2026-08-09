package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/bocacorazon/hermoso/internal/dispatch"
	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/repository"
	"github.com/bocacorazon/hermoso/internal/state"
	"github.com/bocacorazon/hermoso/internal/testutil"
)

var testTime = time.Date(2026, 8, 2, 15, 0, 0, 0, time.UTC)

func TestDesignConstructionFakeKanbanEndToEnd(t *testing.T) {
	t.Parallel()
	repo, service, execution, profile := setup(t, "feature-flow")
	board := newFakeKanban()
	design := testDesign(execution, 1, "Build flow")
	run, changed, err := service.PutDesign(context.Background(), execution, encode(t, design))
	if err != nil || !changed {
		t.Fatalf("put design: changed=%v err=%v", changed, err)
	}
	hash := run.Design.Hash
	if _, changed, err := service.PutDesign(context.Background(), execution, encode(t, design)); err != nil || changed {
		t.Fatalf("idempotent design retry: changed=%v err=%v", changed, err)
	}
	run, changed, err = service.ApproveDesign(context.Background(), execution, 1, hash, "developer", "")
	if err != nil || !changed || run.Phase != domain.PhaseConstruction {
		t.Fatalf("approve: run=%#v changed=%v err=%v", run, changed, err)
	}

	graph := testGraph(execution, []domain.WorkItem{
		testItem("join", []string{"left", "right"}),
		testItem("right", nil),
		testItem("left", nil),
	})
	if _, _, err := service.PutGraph(context.Background(), execution, encode(t, graph)); err != nil {
		t.Fatal(err)
	}
	run, plan, changed, err := service.Prepare(context.Background(), execution, profile)
	if err != nil || !changed || len(plan.Cards) != 3 {
		t.Fatalf("prepare: cards=%d changed=%v err=%v", len(plan.Cards), changed, err)
	}
	if _, _, changed, err := service.Prepare(context.Background(), execution, profile); err != nil || changed {
		t.Fatalf("interrupted prepare retry: changed=%v err=%v", changed, err)
	}
	ready, err := service.Ready(context.Background(), execution)
	if err != nil || !reflect.DeepEqual(cardIDs(ready), []string{"left", "right"}) {
		t.Fatalf("initial ready=%v err=%v", cardIDs(ready), err)
	}

	for _, card := range ready {
		id := card.Identity.WorkItemID
		taskID, created := board.Create(card)
		if !created || taskID != "task-"+id {
			t.Fatalf("create %s: task=%s created=%v", id, taskID, created)
		}
		if retryID, created := board.Create(card); created || retryID != taskID {
			t.Fatalf("idempotent Kanban create %s: task=%s created=%v", id, retryID, created)
		}
		if _, created, err := service.BindTask(context.Background(), execution, id, taskID); err != nil || !created {
			t.Fatalf("bind %s: created=%v err=%v", id, created, err)
		}
		if _, created, err := service.BindTask(context.Background(), execution, id, taskID); err != nil || created {
			t.Fatalf("orphan recovery bind %s: created=%v err=%v", id, created, err)
		}
		if _, _, err := service.StartWork(context.Background(), execution, id); err != nil {
			t.Fatal(err)
		}
		item := itemState(t, service, execution, id)
		gitAt(t, item.Workspace.Path, "commit", "--allow-empty", "-m", id)
		evidence := testEvidence(execution, "evidence-"+id)
		if _, _, err := service.FinishWork(context.Background(), execution, id, domain.WorkCompleted, evidence, ""); err != nil {
			t.Fatal(err)
		}
	}
	ready, err = service.Ready(context.Background(), execution)
	if err != nil || !reflect.DeepEqual(cardIDs(ready), []string{"join"}) {
		t.Fatalf("fan-in ready=%v err=%v", cardIDs(ready), err)
	}
	if got := []string{ready[0].Parents[0].TaskID, ready[0].Parents[1].TaskID}; !reflect.DeepEqual(got, []string{"task-left", "task-right"}) {
		t.Fatalf("parent task IDs=%v", got)
	}
	joinTask, created := board.Create(ready[0])
	if !created || joinTask != "task-join" {
		t.Fatalf("create join: task=%s created=%v", joinTask, created)
	}
	if _, _, err := service.BindTask(context.Background(), execution, "join", joinTask); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.StartWork(context.Background(), execution, "join"); err != nil {
		t.Fatal(err)
	}
	join := itemState(t, service, execution, "join")
	for _, name := range []string{"left", "right"} {
		if !gitAncestor(repo, "hermoso/run/"+execution.RunID+"/work/feature-flow/"+name, join.Workspace.Branch) {
			t.Errorf("%s was not synchronized into join", name)
		}
	}
	gitAt(t, join.Workspace.Path, "commit", "--allow-empty", "-m", "join")
	if _, _, err := service.FinishWork(context.Background(), execution, "join", domain.WorkCompleted, testEvidence(execution, "evidence-join"), ""); err != nil {
		t.Fatal(err)
	}
	run, integration, _, err := service.Integrate(context.Background(), execution, []string{"git status --porcelain"})
	if err != nil || integration.Block != nil || run.Construction.IntegratedAt == nil ||
		run.Construction.IntegratedFeatureCommit == "" || len(run.Construction.IntegratedLeaves) != 1 {
		t.Fatalf("integrate: result=%#v run=%#v err=%v", integration, run, err)
	}
	result := testResult(execution, domain.ResultCompleted, nil)
	run, _, err = service.PutResult(context.Background(), execution, encode(t, result))
	if err != nil || run.Status != domain.StatusAwaitingVerification {
		t.Fatalf("result: status=%s err=%v", run.Status, err)
	}
	if got := board.TaskIDs(); !reflect.DeepEqual(got, []string{"task-join", "task-left", "task-right"}) {
		t.Fatalf("fake Kanban tasks=%v", got)
	}
}

func TestApprovalHashInvalidationAndContextIsolation(t *testing.T) {
	t.Parallel()
	_, serviceA, contextA, _ := setup(t, "feature-a")
	_, serviceB, contextB, _ := setup(t, "feature-b")

	first := testDesign(contextA, 1, "First")
	run, _, err := serviceA.PutDesign(context.Background(), contextA, encode(t, first))
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := serviceA.ApproveDesign(context.Background(), contextA, 1, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "dev", ""); err == nil {
		t.Fatal("stale hash approval accepted")
	}
	if _, _, err := serviceA.ApproveDesign(context.Background(), contextA, 1, run.Design.Hash, "dev", ""); err != nil {
		t.Fatal(err)
	}
	revised := testDesign(contextA, 2, "Revised")
	run, changed, err := serviceA.PutDesign(context.Background(), contextA, encode(t, revised))
	if err != nil || !changed || run.Design.Approval != nil || run.Status != domain.StatusAwaitingApproval {
		t.Fatalf("revision did not invalidate approval: run=%#v changed=%v err=%v", run, changed, err)
	}
	if _, _, err := serviceA.ApproveDesign(context.Background(), contextA, 1, run.Design.Hash, "dev", ""); err == nil {
		t.Fatal("old revision approval accepted after design change")
	}
	if _, _, err := serviceB.PutDesign(context.Background(), contextB, encode(t, first)); err == nil {
		t.Fatal("cross-project design context accepted")
	}
	mismatch := contextA
	mismatch.FeatureID = contextB.FeatureID
	if _, _, err := serviceA.PutDesign(context.Background(), mismatch, encode(t, first)); err == nil {
		t.Fatal("mismatched canonical context accepted")
	}
}

func TestSameRepositoryFeatureAndRunContextsRemainIsolated(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	if _, _, err := state.Initialize(context.Background(), repo.Root, testTime); err != nil {
		t.Fatal(err)
	}
	store, _, err := state.Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	runA, err := store.StartRun(context.Background(), "feature-shared", testTime)
	if err != nil {
		t.Fatal(err)
	}
	runB, err := store.StartRun(context.Background(), "feature-shared", testTime.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	runC, err := store.StartRun(context.Background(), "feature-other", testTime.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := repository.NewManager(repo.Root, filepath.Join(repo.Root, ".hermoso", "worktrees"), "hermoso")
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Store: store, Manager: manager, Now: func() time.Time { return testTime.Add(time.Minute) }}

	for _, execution := range []domain.ContextRef{runA.Context, runB.Context, runC.Context} {
		if _, _, err := service.PutDesign(context.Background(), execution, encode(t, testDesign(execution, 1, execution.RunID))); err != nil {
			t.Fatal(err)
		}
	}
	for _, execution := range []domain.ContextRef{runA.Context, runB.Context, runC.Context} {
		persisted, err := store.Run(context.Background(), execution)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.Design.Contract.Feature.Objective != execution.RunID {
			t.Fatalf("context %s read another run's design: %#v", execution.RunID, persisted.Design.Contract)
		}
	}

	crossRun := testDesign(runA.Context, 2, "wrong run")
	crossRun.Context.RunID = runB.Context.RunID
	if _, _, err := service.PutDesign(context.Background(), runA.Context, encode(t, crossRun)); err == nil {
		t.Fatal("cross-run design accepted")
	}
	crossFeature := testDesign(runA.Context, 2, "wrong feature")
	crossFeature.Context.FeatureID = runC.Context.FeatureID
	if _, _, err := service.PutDesign(context.Background(), runA.Context, encode(t, crossFeature)); err == nil {
		t.Fatal("cross-feature design accepted")
	}
}

func TestBlockResumePreservesConflictResolution(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setup(t, "feature-conflict")
	run, _, err := service.PutDesign(context.Background(), execution, encode(t, testDesign(execution, 1, "Conflict")))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ApproveDesign(context.Background(), execution, 1, run.Design.Hash, "dev", ""); err != nil {
		t.Fatal(err)
	}
	graph := testGraph(execution, []domain.WorkItem{
		testItem("join", []string{"left", "right"}),
		testItem("left", nil), testItem("right", nil),
	})
	if _, _, err := service.PutGraph(context.Background(), execution, encode(t, graph)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Prepare(context.Background(), execution, profile); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"left", "right"} {
		if _, _, err := service.BindTask(context.Background(), execution, id, "task-"+id); err != nil {
			t.Fatal(err)
		}
		if _, _, err := service.StartWork(context.Background(), execution, id); err != nil {
			t.Fatal(err)
		}
		item := itemState(t, service, execution, id)
		if err := os.WriteFile(filepath.Join(item.Workspace.Path, "README.md"), []byte(id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitAt(t, item.Workspace.Path, "add", "README.md")
		gitAt(t, item.Workspace.Path, "commit", "-m", id)
		if _, _, err := service.FinishWork(context.Background(), execution, id, domain.WorkCompleted, testEvidence(execution, "ev-"+id), ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := service.BindTask(context.Background(), execution, "join", "task-join"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.StartWork(context.Background(), execution, "join"); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("start conflict error=%v", err)
	}
	run, err = service.Store.Run(context.Background(), execution)
	if err != nil || run.Status != domain.StatusBlocked {
		t.Fatalf("blocked run=%#v err=%v", run, err)
	}
	join := itemState(t, service, execution, "join")
	if err := os.WriteFile(filepath.Join(join.Workspace.Path, "README.md"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAt(t, join.Workspace.Path, "add", "README.md")
	if _, changed, err := service.Resume(context.Background(), execution); err != nil || !changed {
		t.Fatalf("resume changed=%v err=%v", changed, err)
	}
	if _, _, err := service.StartWork(context.Background(), execution, "join"); err != nil {
		t.Fatalf("continued synchronized merge: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(join.Workspace.Path, "README.md"))
	if err != nil || string(data) != "resolved\n" {
		t.Fatalf("user resolution was reset: %q err=%v", data, err)
	}
}

func TestBlockedWorkResumesWithBindingAndCompletionRetryIntact(t *testing.T) {
	t.Parallel()
	_, service, execution, profile := setup(t, "feature-blocked")
	run, _, err := service.PutDesign(context.Background(), execution, encode(t, testDesign(execution, 1, "Blocked work")))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ApproveDesign(context.Background(), execution, 1, run.Design.Hash, "dev", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.PutGraph(context.Background(), execution, encode(t, testGraph(execution, []domain.WorkItem{testItem("root", nil)}))); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Prepare(context.Background(), execution, profile); err != nil {
		t.Fatal(err)
	}
	binding, created, err := service.BindTask(context.Background(), execution, "root", "task-root")
	if err != nil || !created {
		t.Fatalf("bind: %#v created=%v err=%v", binding, created, err)
	}
	if _, _, err := service.StartWork(context.Background(), execution, "root"); err != nil {
		t.Fatal(err)
	}
	blockedEvidence := testEvidence(execution, "blocked-root")
	if _, changed, err := service.FinishWork(context.Background(), execution, "root", domain.WorkBlocked, blockedEvidence, "dependency unavailable"); err != nil || !changed {
		t.Fatalf("block: changed=%v err=%v", changed, err)
	}
	if _, changed, err := service.Resume(context.Background(), execution); err != nil || !changed {
		t.Fatalf("resume: changed=%v err=%v", changed, err)
	}
	if _, changed, err := service.Resume(context.Background(), execution); err != nil || changed {
		t.Fatalf("idempotent resume: changed=%v err=%v", changed, err)
	}
	if ready, err := service.Ready(context.Background(), execution); err != nil || len(ready) != 0 {
		t.Fatalf("bound card was recreated after resume: ready=%v err=%v", ready, err)
	}
	if _, _, err := service.StartWork(context.Background(), execution, "root"); err != nil {
		t.Fatal(err)
	}
	item := itemState(t, service, execution, "root")
	gitAt(t, item.Workspace.Path, "commit", "--allow-empty", "-m", "root")
	completedEvidence := testEvidence(execution, "completed-root")
	if _, changed, err := service.FinishWork(context.Background(), execution, "root", domain.WorkCompleted, completedEvidence, ""); err != nil || !changed {
		t.Fatalf("complete: changed=%v err=%v", changed, err)
	}
	if _, changed, err := service.FinishWork(context.Background(), execution, "root", domain.WorkCompleted, completedEvidence, ""); err != nil || changed {
		t.Fatalf("idempotent completion: changed=%v err=%v", changed, err)
	}
}

func TestPreparePersistsCanonicalProfilePathAcrossWorkingDirectoryChanges(t *testing.T) {
	repo, service, execution, _ := setup(t, "profile-recovery")
	run, _, err := service.PutDesign(context.Background(), execution, encode(t, testDesign(execution, 1, "Profile recovery")))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ApproveDesign(context.Background(), execution, 1, run.Design.Hash, "dev", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.PutGraph(context.Background(), execution, encode(t, testGraph(execution, []domain.WorkItem{testItem("root", nil)}))); err != nil {
		t.Fatal(err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(original)
	if err := os.Chdir(repo.Root); err != nil {
		t.Fatal(err)
	}
	run, _, _, err = service.Prepare(context.Background(), execution, "profile.json")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(run.Construction.ProfilePath) {
		t.Fatalf("persisted profile path = %q, want absolute", run.Construction.ProfilePath)
	}
	if err := os.Chdir(filepath.Dir(repo.Root)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Ready(context.Background(), execution); err != nil {
		t.Fatalf("ready after cwd change: %v", err)
	}
}

func TestCompletedConstructionIsImmutable(t *testing.T) {
	_, service, execution, profile := setup(t, "immutable-construction")
	run := prepareCompletedLeaf(t, service, execution, profile)
	if _, _, _, err := service.Integrate(context.Background(), execution, nil); err != nil {
		t.Fatal(err)
	}
	result := testResult(execution, domain.ResultCompleted, nil)
	run, _, err := service.PutResult(context.Background(), execution, encode(t, result))
	if err != nil || run.Status != domain.StatusAwaitingVerification {
		t.Fatalf("put result: status=%s err=%v", run.Status, err)
	}

	graph := testGraph(execution, []domain.WorkItem{testItem("root", nil)})
	graph.Revision = 2
	for name, mutate := range map[string]func() error{
		"graph": func() error {
			_, _, err := service.PutGraph(context.Background(), execution, encode(t, graph))
			return err
		},
		"prepare": func() error {
			_, _, _, err := service.Prepare(context.Background(), execution, profile)
			return err
		},
		"bind": func() error {
			_, _, err := service.BindTask(context.Background(), execution, "root", "task-other")
			return err
		},
		"start": func() error {
			_, _, err := service.StartWork(context.Background(), execution, "root")
			return err
		},
		"finish": func() error {
			_, _, err := service.FinishWork(context.Background(), execution, "root", domain.WorkCompleted, testEvidence(execution, "late"), "")
			return err
		},
		"resume": func() error {
			_, _, err := service.Resume(context.Background(), execution)
			return err
		},
		"integrate": func() error {
			_, _, _, err := service.Integrate(context.Background(), execution, nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := mutate(); err == nil || !strings.Contains(err.Error(), "immutable") {
				t.Fatalf("mutation error = %v, want immutable rejection", err)
			}
		})
	}
}

func TestIntegrationAndResultRejectCommitDrift(t *testing.T) {
	tests := []struct {
		name  string
		drift func(domain.Run)
	}{
		{"leaf", func(run domain.Run) {
			gitAt(t, run.Construction.Items[0].Workspace.Path, "commit", "--allow-empty", "-m", "leaf drift")
		}},
		{"feature", func(run domain.Run) {
			gitAt(t, run.Construction.Feature.Path, "commit", "--allow-empty", "-m", "feature drift")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, service, execution, profile := setup(t, "integration-drift-"+test.name)
			run := prepareCompletedLeaf(t, service, execution, profile)
			run, _, _, err := service.Integrate(context.Background(), execution, nil)
			if err != nil {
				t.Fatal(err)
			}
			test.drift(run)

			if _, _, _, err := service.Integrate(context.Background(), execution, nil); err == nil || !strings.Contains(err.Error(), "drifted") {
				t.Fatalf("reintegration drift error = %v", err)
			}
			if _, _, err := service.PutResult(context.Background(), execution, encode(t, testResult(execution, domain.ResultCompleted, nil))); err == nil || !strings.Contains(err.Error(), "drifted") {
				t.Fatalf("result drift error = %v", err)
			}
		})
	}
}

func prepareCompletedLeaf(t *testing.T, service Service, execution domain.ContextRef, profile string) domain.Run {
	t.Helper()
	run, _, err := service.PutDesign(context.Background(), execution, encode(t, testDesign(execution, 1, "Complete leaf")))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ApproveDesign(context.Background(), execution, 1, run.Design.Hash, "dev", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.PutGraph(context.Background(), execution, encode(t, testGraph(execution, []domain.WorkItem{testItem("root", nil)}))); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Prepare(context.Background(), execution, profile); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.BindTask(context.Background(), execution, "root", "task-root"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.StartWork(context.Background(), execution, "root"); err != nil {
		t.Fatal(err)
	}
	item := itemState(t, service, execution, "root")
	gitAt(t, item.Workspace.Path, "commit", "--allow-empty", "-m", "root")
	run, _, err = service.FinishWork(context.Background(), execution, "root", domain.WorkCompleted, testEvidence(execution, "root-done"), "")
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestReadyWithholdsChildWhenCompletedParentHasNoTaskBinding(t *testing.T) {
	t.Parallel()

	_, service, execution, profile := setup(t, "unbound-parent")
	design := testDesign(execution, 1, "Verify binding readiness")
	run, _, err := service.PutDesign(context.Background(), execution, encode(t, design))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ApproveDesign(context.Background(), execution, design.Revision, run.Design.Hash, "developer", ""); err != nil {
		t.Fatal(err)
	}
	graph := testGraph(execution, []domain.WorkItem{
		testItem("parent", nil),
		testItem("child", []string{"parent"}),
	})
	if _, _, err := service.PutGraph(context.Background(), execution, encode(t, graph)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Prepare(context.Background(), execution, profile); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.StartWork(context.Background(), execution, "parent"); err != nil {
		t.Fatal(err)
	}
	parent := itemState(t, service, execution, "parent")
	gitAt(t, parent.Workspace.Path, "commit", "--allow-empty", "-m", "parent")
	if _, _, err := service.FinishWork(context.Background(), execution, "parent", domain.WorkCompleted, testEvidence(execution, "parent-evidence"), ""); err != nil {
		t.Fatal(err)
	}

	ready, err := service.Ready(context.Background(), execution)
	if err != nil {
		t.Fatalf("ready with unbound completed parent: %v", err)
	}
	if len(ready) != 0 {
		t.Fatalf("ready cards = %v, want child withheld until parent task binding exists", cardIDs(ready))
	}
}

func setup(t *testing.T, feature string) (*testutil.Repository, Service, domain.ContextRef, string) {
	t.Helper()
	repo := testutil.NewRepository(t)
	if _, _, err := state.Initialize(context.Background(), repo.Root, testTime); err != nil {
		t.Fatal(err)
	}
	store, _, err := state.Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartRun(context.Background(), feature, testTime)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := repository.NewManager(repo.Root, filepath.Join(repo.Root, ".hermoso", "worktrees"), "hermoso")
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(repo.Root, "profile.json")
	repo.Write("profile.json", `{"phases":{"integration":{"profile":"default","skills":["hermoso-construction"],"goal_mode":"integration"}},"dispatch":{"priority":0,"default_runtime_budget_seconds":900}}`)
	return repo, Service{Store: store, Manager: manager, Now: func() time.Time { return testTime.Add(time.Minute) }}, run.Context, profile
}

func testDesign(ctx domain.ContextRef, revision uint64, objective string) domain.FeatureDesign {
	return domain.FeatureDesign{
		SchemaVersion: domain.SchemaVersion, Context: ctx,
		Producer: domain.Producer{Skill: "hermoso-design", Runtime: "test"}, Revision: revision,
		Feature:            domain.FeatureIdentity{ID: ctx.FeatureID, Title: "Feature", Objective: objective, TargetRepository: ctx.Repository},
		AcceptanceCriteria: []string{"Works"}, UnresolvedQuestions: []string{}, Complexity: domain.ComplexityStandard,
	}
}

func testGraph(ctx domain.ContextRef, items []domain.WorkItem) domain.WorkGraph {
	return domain.WorkGraph{
		SchemaVersion: domain.SchemaVersion, Context: ctx,
		Producer: domain.Producer{Skill: "hermoso-construction", Runtime: "test"},
		Revision: 1, Items: items,
	}
}

func testItem(id string, parents []string) domain.WorkItem {
	return domain.WorkItem{
		ID: id, Title: id, Prompt: "Implement " + id, Parents: parents,
		AcceptanceCriteria: []string{id + " works"},
		Worker:             domain.Worker{Profile: "default", Skills: []domain.SkillBinding{{Name: "hermoso-construction"}}},
	}
}

func testEvidence(ctx domain.ContextRef, id string) domain.Evidence {
	return domain.Evidence{Context: ctx, ID: id, Kind: "command", Command: "go test ./...", Summary: "passed", RecordedAt: testTime}
}

func testResult(ctx domain.ContextRef, status domain.ResultStatus, blockers []string) domain.PhaseResult {
	return domain.PhaseResult{
		SchemaVersion: domain.SchemaVersion, Context: ctx,
		Producer: domain.Producer{Skill: "hermoso-construction", Runtime: "test"},
		Phase:    domain.PhaseConstruction, Status: status, Summary: "construction done",
		Blockers: blockers, CompletedAt: testTime,
	}
}

func itemState(t *testing.T, service Service, ctx domain.ContextRef, id string) domain.WorkState {
	t.Helper()
	run, err := service.Store.Run(context.Background(), ctx)
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := findItem(run, id)
	if err != nil {
		t.Fatal(err)
	}
	return *item
}

func cardIDs(cards []dispatch.Card) []string {
	ids := make([]string, len(cards))
	for i := range cards {
		ids[i] = cards[i].Identity.WorkItemID
	}
	return ids
}

func encode(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func gitAncestor(repo *testutil.Repository, ancestor, descendant string) bool {
	cmd := exec.Command("git", "-C", repo.Root, "merge-base", "--is-ancestor", ancestor, descendant)
	return cmd.Run() == nil
}

func gitAt(t *testing.T, path string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

type fakeKanban struct {
	taskByKey  map[string]string
	cardByTask map[string]dispatch.Card
}

func newFakeKanban() *fakeKanban {
	return &fakeKanban{taskByKey: map[string]string{}, cardByTask: map[string]dispatch.Card{}}
}

func (f *fakeKanban) Create(card dispatch.Card) (string, bool) {
	if taskID, ok := f.taskByKey[card.IdempotencyKey]; ok {
		return taskID, false
	}
	taskID := "task-" + card.Identity.WorkItemID
	f.taskByKey[card.IdempotencyKey] = taskID
	f.cardByTask[taskID] = card
	return taskID, true
}

func (f *fakeKanban) TaskIDs() []string {
	ids := make([]string, 0, len(f.cardByTask))
	for id := range f.cardByTask {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
