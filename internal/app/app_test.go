package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/state"
	"github.com/bocacorazon/hermoso/internal/testutil"
)

func TestRunTextHelp(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), nil, testDependencies(&stdout, &stderr))

	if exitCode != ExitOK {
		t.Fatalf("exit code = %d, want %d", exitCode, ExitOK)
	}
	if !strings.Contains(stdout.String(), "TUI-first") {
		t.Errorf("stdout does not contain product boundary: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunJSONVersion(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{"version", "--json"},
		testDependencies(&stdout, &stderr),
	)

	if exitCode != ExitOK {
		t.Fatalf("exit code = %d, want %d", exitCode, ExitOK)
	}
	var got response
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !got.OK || got.Command != "version" || got.Data["version"] != "test" {
		t.Errorf("response = %#v", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunJSONUsageError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{"not-a-command", "--json"},
		testDependencies(&stdout, &stderr),
	)

	if exitCode != ExitUsage {
		t.Fatalf("exit code = %d, want %d", exitCode, ExitUsage)
	}
	var got response
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got.OK || got.Error == nil || got.Error.Code != ErrorUsage {
		t.Errorf("response = %#v", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty for a JSON response", stderr.String())
	}
}

func TestRunSchemaCommands(t *testing.T) {
	t.Parallel()

	var textOut, textErr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{"schema", "feature-design"},
		testDependencies(&textOut, &textErr),
	)
	if exitCode != ExitOK {
		t.Fatalf("text exit code = %d, stderr = %q", exitCode, textErr.String())
	}
	var schema map[string]any
	if err := json.Unmarshal(textOut.Bytes(), &schema); err != nil {
		t.Fatalf("text schema is not JSON: %v", err)
	}
	if schema["$id"] != "https://hermoso.dev/schemas/v2/feature-design.json" {
		t.Errorf("$id = %v", schema["$id"])
	}

	var jsonOut, jsonErr bytes.Buffer
	exitCode = Run(
		context.Background(),
		[]string{"--json", "schema", "work-graph"},
		testDependencies(&jsonOut, &jsonErr),
	)
	if exitCode != ExitOK {
		t.Fatalf("JSON exit code = %d, stderr = %q", exitCode, jsonErr.String())
	}
	var got response
	if err := json.Unmarshal(jsonOut.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Command != "schema" || got.Data["schema"] == nil {
		t.Errorf("response = %#v", got)
	}
}

func TestRunValidateCommands(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	var setupOut, setupErr bytes.Buffer
	if code := Run(context.Background(), []string{"init", repo.Root, "--json"}, testDependencies(&setupOut, &setupErr)); code != ExitOK {
		t.Fatalf("init exit code = %d, stderr = %q", code, setupErr.String())
	}
	setupOut.Reset()
	if code := Run(context.Background(), []string{"start", "feature-1", repo.Root, "--json"}, testDependencies(&setupOut, &setupErr)); code != ExitOK {
		t.Fatalf("start exit code = %d, stderr = %q", code, setupErr.String())
	}
	var started response
	if err := json.Unmarshal(setupOut.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	run := started.Data["run"].(map[string]any)
	execution := run["context"].(map[string]any)
	runID := execution["run_id"].(string)

	writeContextFixture(t, repo, "../contracts/testdata/feature-design.valid.json", "feature.json", execution)
	var validOut, validErr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{"validate", "feature-design", filepath.Join(repo.Root, "feature.json"),
			execution["project_id"].(string), execution["feature_id"].(string), runID, repo.Root},
		testDependencies(&validOut, &validErr),
	)
	if exitCode != ExitOK {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, validErr.String())
	}
	if !strings.Contains(validOut.String(), "valid feature-design contract") {
		t.Errorf("stdout = %q", validOut.String())
	}

	writeContextFixture(t, repo, "../contracts/testdata/work-graph.invalid.json", "invalid.json", execution)
	var invalidOut, invalidErr bytes.Buffer
	exitCode = Run(
		context.Background(),
		[]string{"validate", "work-graph", filepath.Join(repo.Root, "invalid.json"),
			execution["project_id"].(string), execution["feature_id"].(string), runID, repo.Root, "--json"},
		testDependencies(&invalidOut, &invalidErr),
	)
	if exitCode != ExitFailure {
		t.Fatalf("invalid exit code = %d", exitCode)
	}
	var got response
	if err := json.Unmarshal(invalidOut.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OK || got.Error == nil || got.Error.Code != ErrorValidation || len(got.Error.Details) == 0 {
		t.Errorf("response = %#v", got)
	}
	if invalidErr.Len() != 0 {
		t.Errorf("stderr = %q", invalidErr.String())
	}
}

func TestRunInitStartAndStatusFromSubdirectory(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	subdirectory := repo.Mkdir("nested/deep")

	var initOut, initErr bytes.Buffer
	exitCode := Run(context.Background(), []string{"init", subdirectory, "--json"}, testDependencies(&initOut, &initErr))
	if exitCode != ExitOK {
		t.Fatalf("init exit code = %d, stderr = %q", exitCode, initErr.String())
	}

	var startOut, startErr bytes.Buffer
	exitCode = Run(context.Background(), []string{"start", "feature-1", subdirectory, "--json"}, testDependencies(&startOut, &startErr))
	if exitCode != ExitOK {
		t.Fatalf("start exit code = %d, stderr = %q", exitCode, startErr.String())
	}

	var statusOut, statusErr bytes.Buffer
	exitCode = Run(context.Background(), []string{"status", subdirectory, "--json"}, testDependencies(&statusOut, &statusErr))
	if exitCode != ExitOK {
		t.Fatalf("status exit code = %d, stderr = %q", exitCode, statusErr.String())
	}
	var got response
	if err := json.Unmarshal(statusOut.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	runs, ok := got.Data["runs"].([]any)
	if !ok || len(runs) != 1 {
		t.Errorf("runs = %#v, want one run", got.Data["runs"])
	}

	run := runs[0].(map[string]any)
	execution := run["context"].(map[string]any)
	var contextOut, contextErr bytes.Buffer
	exitCode = Run(context.Background(), []string{
		"context", execution["project_id"].(string), execution["feature_id"].(string),
		execution["run_id"].(string), repo.Root, "--json",
	}, testDependencies(&contextOut, &contextErr))
	if exitCode != ExitOK {
		t.Fatalf("context exit code = %d, stderr = %q", exitCode, contextErr.String())
	}
	var contextResponse response
	if err := json.Unmarshal(contextOut.Bytes(), &contextResponse); err != nil {
		t.Fatal(err)
	}
	if contextResponse.Data["context"].(map[string]any)["project_id"] != execution["project_id"] {
		t.Errorf("context lookup = %#v, want %#v", contextResponse.Data["context"], execution)
	}
}

func TestRunStatusReportsUninitializedState(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepository(t)
	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"status", repo.Root, "--json"}, testDependencies(&stdout, &stderr))
	if exitCode != ExitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, ExitFailure)
	}
	var got response
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error == nil || got.Error.Code != "not_initialized" {
		t.Errorf("response = %#v", got)
	}
}

func TestRunDesignConstructionCommands(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	depsOut, depsErr := &bytes.Buffer{}, &bytes.Buffer{}
	deps := testDependencies(depsOut, depsErr)
	if code := Run(context.Background(), []string{"init", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("init: code=%d output=%s", code, depsOut.String())
	}

	depsOut.Reset()
	if code := Run(context.Background(), []string{"start", "feature-cli", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("start: code=%d output=%s", code, depsOut.String())
	}
	var started response
	if err := json.Unmarshal(depsOut.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	execution := started.Data["run"].(map[string]any)["context"].(map[string]any)
	full := []string{
		execution["project_id"].(string), execution["feature_id"].(string),
		execution["run_id"].(string), repo.Root,
	}
	writeContextFixture(t, repo, "../contracts/testdata/feature-design.valid.json", "design.json", execution)
	seedModelForDesign(t, repo, execution, filepath.Join(repo.Root, "design.json"))
	depsOut.Reset()
	args := append([]string{"design", "put"}, full...)
	args = append(args, filepath.Join(repo.Root, "design.json"), "--json")
	if code := Run(context.Background(), args, deps); code != ExitOK {
		t.Fatalf("design put: code=%d output=%s", code, depsOut.String())
	}
	var persisted response
	if err := json.Unmarshal(depsOut.Bytes(), &persisted); err != nil {
		t.Fatal(err)
	}
	design := persisted.Data["design"].(map[string]any)
	writeVerificationFixture(t, repo, design)
	depsOut.Reset()
	args = append([]string{"verification", "put"}, full...)
	args = append(args, filepath.Join(repo.Root, "verification.json"), "--json")
	if code := Run(context.Background(), args, deps); code != ExitOK {
		t.Fatalf("verification put: code=%d output=%s", code, depsOut.String())
	}
	var verification response
	if err := json.Unmarshal(depsOut.Bytes(), &verification); err != nil {
		t.Fatal(err)
	}
	design = verification.Data["design"].(map[string]any)
	depsOut.Reset()
	args = append([]string{"approve", "design"}, full...)
	args = append(
		args,
		fmt.Sprintf("%.0f", design["revision"].(float64)),
		design["package_hash"].(string),
		"cli-test",
		"--json",
	)
	if code := Run(context.Background(), args, deps); code != ExitOK {
		t.Fatalf("approve: code=%d output=%s", code, depsOut.String())
	}
	writeContextFixture(t, repo, "../contracts/testdata/work-graph.valid.json", "graph.json", execution)
	depsOut.Reset()
	args = append([]string{"graph", "put"}, full...)
	args = append(args, filepath.Join(repo.Root, "graph.json"), "--json")
	if code := Run(context.Background(), args, deps); code != ExitOK {
		t.Fatalf("graph put: code=%d output=%s", code, depsOut.String())
	}
	profile, err := filepath.Abs("../../profiles/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	depsOut.Reset()
	args = append([]string{"construction", "prepare"}, full...)
	args = append(args, profile, "--json")
	if code := Run(context.Background(), args, deps); code != ExitOK {
		t.Fatalf("prepare: code=%d output=%s", code, depsOut.String())
	}
	depsOut.Reset()
	args = append([]string{"construction", "ready"}, full...)
	args = append(args, "--json")
	if code := Run(context.Background(), args, deps); code != ExitOK {
		t.Fatalf("ready: code=%d output=%s", code, depsOut.String())
	}
	var ready response
	if err := json.Unmarshal(depsOut.Bytes(), &ready); err != nil {
		t.Fatal(err)
	}
	if cards := ready.Data["cards"].([]any); len(cards) == 0 {
		t.Fatal("no create-ready cards emitted")
	}
}

func TestRunModelCommandsReportFreshnessAndQueries(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	deps := testDependencies(stdout, stderr)
	if code := Run(context.Background(), []string{"init", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("init: code=%d output=%s", code, stdout.String())
	}
	var initialized response
	if err := json.Unmarshal(stdout.Bytes(), &initialized); err != nil {
		t.Fatal(err)
	}
	projectID := initialized.Data["project"].(map[string]any)["project_id"].(string)

	stdout.Reset()
	if code := Run(context.Background(), []string{"model", "build", projectID, repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("model build: code=%d output=%s", code, stdout.String())
	}
	var built response
	if err := json.Unmarshal(stdout.Bytes(), &built); err != nil {
		t.Fatal(err)
	}
	if built.Data["snapshot"].(map[string]any)["snapshot_id"] == "" {
		t.Fatalf("build response = %#v", built)
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"model", "query", projectID, repo.Root, "orientation", "--json"}, deps); code != ExitOK {
		t.Fatalf("model query: code=%d output=%s", code, stdout.String())
	}
	var queried response
	if err := json.Unmarshal(stdout.Bytes(), &queried); err != nil {
		t.Fatal(err)
	}
	if queried.Data["fresh"] != true || queried.Data["result"] == nil {
		t.Fatalf("query response = %#v", queried)
	}

	repo.Write("CHANGE.md", "# Change\n")
	repo.Commit("change repository")
	stdout.Reset()
	if code := Run(context.Background(), []string{"model", "status", projectID, repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("model status: code=%d output=%s", code, stdout.String())
	}
	var status response
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	modelStatus := status.Data["model"].(map[string]any)
	if modelStatus["fresh"] != false || len(modelStatus["stale_reasons"].([]any)) == 0 {
		t.Fatalf("status response = %#v", status)
	}
}

func testDependencies(stdout, stderr *bytes.Buffer) Dependencies {
	return Dependencies{
		Stdin:   strings.NewReader(""),
		Stdout:  stdout,
		Stderr:  stderr,
		FS:      OSFilesystem{},
		Version: "test",
		Now:     func() time.Time { return time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC) },
	}

}

func writeContextFixture(t *testing.T, repo *testutil.Repository, source, destination string, execution map[string]any) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	document["context"] = execution
	if feature, ok := document["feature"].(map[string]any); ok {
		feature["id"] = execution["feature_id"]
		feature["target_repository"] = execution["repository"]
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	repo.Write(destination, string(encoded)+"\n")
}

func seedModelForDesign(
	t *testing.T,
	repo *testutil.Repository,
	execution map[string]any,
	designPath string,
) {
	t.Helper()
	var contextRef domain.ContextRef
	decodeMap(t, execution, &contextRef)
	store, _, err := state.Load(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := testutil.ModelSnapshot(t, contextRef)
	if _, _, err := store.PutModelSnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatal(err)
	}
	var design map[string]any
	if err := json.Unmarshal(data, &design); err != nil {
		t.Fatal(err)
	}
	var reference map[string]any
	decodeMap(t, testutil.ModelReference(snapshot), &reference)
	design["base_model"] = reference
	encoded, err := json.MarshalIndent(design, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(designPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeVerificationFixture(t *testing.T, repo *testutil.Repository, designState map[string]any) {
	t.Helper()
	var design domain.FeatureDesign
	decodeMap(t, designState["feature_design"], &design)
	feature := []byte(`@requirement:req-reject-malformed @requirement:req-accept-valid @criterion:ac-reject-malformed @criterion:ac-accept-valid @surface:surface-schema-cli @term:term-contract
Feature: Validate phase contracts

  @scenario:scenario-validate-contract @judgment:judgment-validate-contract
  Scenario: Distinguish valid and malformed contracts
    Given a phase contract
    When the contract is validated
    Then valid contracts are accepted and malformed contracts are rejected
`)
	repo.Write("features/contract.feature", string(feature))
	contract := domain.FeatureVerificationContract{
		SchemaVersion: domain.SchemaVersion,
		Context:       design.Context,
		Producer: domain.Producer{
			Skill: "hermoso-verification-author", Runtime: "test",
		},
		Revision: 1,
		FeatureDesign: domain.ContractReference{
			Context: design.Context, Kind: "feature-design", Path: "design.json",
			Revision: design.Revision, Hash: designState["feature_hash"].(string),
		},
		BaseModel: design.BaseModel,
		Artifacts: []domain.VerificationArtifact{{
			ID: "artifact-contract", Kind: "gherkin", Path: "features/contract.feature",
			ContentHash: digest.Bytes(feature), PublicationPath: "features/contract.feature",
		}},
		Judgments: []domain.VerificationJudgment{{
			ID: "judgment-validate-contract", Title: "Validate phase contracts", Modality: "bdd",
			RequirementIDs:         []string{"req-reject-malformed", "req-accept-valid"},
			AcceptanceCriterionIDs: []string{"ac-reject-malformed", "ac-accept-valid"},
			SurfaceIDs:             []string{"surface-schema-cli"},
			BusinessTermIDs:        []string{"term-contract"},
			ArtifactIDs:            []string{"artifact-contract"},
			ScenarioIDs:            []string{"scenario-validate-contract"},
			Execution:              domain.VerificationExecution{Command: []string{"go", "test", "./..."}, TimeoutSeconds: 300},
			Oracle:                 domain.VerificationOracle{Type: "gherkin"},
			RequiredEvidence:       []string{"scenario result", "command output"},
		}},
		Aggregation: domain.VerificationAggregation{Strategy: "all_required"},
	}
	encoded, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	repo.Write("verification.json", string(encoded)+"\n")
}

func decodeMap(t *testing.T, input any, output any) {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, output); err != nil {
		t.Fatal(err)
	}
}
