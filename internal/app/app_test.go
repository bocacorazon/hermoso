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
	if schema["$id"] != "https://hermoso.dev/schemas/v1/feature-design.json" {
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
	contract := design["contract"].(map[string]any)
	depsOut.Reset()
	args = append([]string{"approve", "design"}, full...)
	args = append(args, fmt.Sprintf("%.0f", contract["revision"].(float64)), design["hash"].(string), "cli-test", "--json")
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
