package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bocacorazon/hermoso/internal/testutil"
)

// startVerifiedDesign writes a feature design and its verification contract
// into a fresh repository and persists both. It returns the execution
// context, the contract path, and the design state needed by follow-up
// steps.
func startVerifiedDesign(t *testing.T, repo *testutil.Repository, deps Dependencies, out *bytes.Buffer) (map[string]any, map[string]any, string) {
	t.Helper()
	ctx := context.Background()
	out.Reset()
	if code := Run(ctx, []string{"init", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("init: code=%d output=%s", code, out.String())
	}
	out.Reset()
	if code := Run(ctx, []string{"start", "staging-test", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("start: code=%d output=%s", code, out.String())
	}
	var started response
	if err := json.Unmarshal(out.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	execution := started.Data["run"].(map[string]any)["context"].(map[string]any)
	full := []string{
		execution["project_id"].(string), execution["feature_id"].(string),
		execution["run_id"].(string), repo.Root,
	}
	writeContextFixture(t, repo, "../contracts/testdata/feature-design.valid.json", "design.json", execution)
	seedModelForDesign(t, repo, execution, repo.Path("design.json"))
	out.Reset()
	args := append([]string{"design", "put"}, full...)
	args = append(args, repo.Path("design.json"), "--json")
	if code := Run(ctx, args, deps); code != ExitOK {
		t.Fatalf("design put: code=%d output=%s", code, out.String())
	}
	var persisted response
	if err := json.Unmarshal(out.Bytes(), &persisted); err != nil {
		t.Fatal(err)
	}
	design := persisted.Data["design"].(map[string]any)
	writeVerificationFixture(t, repo, design)
	return execution, design, repo.Path("verification.json")
}

func TestVerificationPutWarnsWhenSealedArtifactsAreNotGitIgnored(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	out, errs := &bytes.Buffer{}, &bytes.Buffer{}
	deps := testDependencies(out, errs)
	execution, _, contractPath := startVerifiedDesign(t, repo, deps, out)

	// No .gitignore: the sealed Gherkin artifact is not ignored, so the
	// ingest must warn that it could be committed.
	out.Reset()
	errs.Reset()
	full := []string{
		execution["project_id"].(string), execution["feature_id"].(string),
		execution["run_id"].(string), repo.Root,
	}
	args := append([]string{"verification", "put"}, full...)
	args = append(args, contractPath, "--json")
	if code := Run(context.Background(), args, deps); code != ExitOK {
		t.Fatalf("verification put: code=%d output=%s stderr=%s", code, out.String(), errs.String())
	}
	if !strings.Contains(errs.String(), "not gitignored") {
		t.Errorf("expected an un-ignored sealed artifact warning, stderr=%q", errs.String())
	}

	// With a .gitignore covering the artifact, no warning.
	repo.Write(".gitignore", "features/\n")
	out.Reset()
	errs.Reset()
	args = append([]string{"verification", "put"}, full...)
	args = append(args, contractPath, "--json")
	if code := Run(context.Background(), args, deps); code != ExitOK {
		t.Fatalf("verification put (ignored): code=%d output=%s stderr=%s", code, out.String(), errs.String())
	}
	if strings.Contains(errs.String(), "not gitignored") {
		t.Errorf("expected no warning for a gitignored artifact, stderr=%q", errs.String())
	}
}

func TestContextConstitutionResolution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	out, errs := &bytes.Buffer{}, &bytes.Buffer{}
	deps := testDependencies(out, errs)

	// Spec Kit location resolves without a docs constitution.
	repo := testutil.NewRepository(t)
	repo.Write(".specify/memory/constitution.md", "# Spec Kit constitution\n")
	out.Reset()
	errs.Reset()
	if code := Run(ctx, []string{"context", "constitution", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("context constitution (specify): code=%d output=%s stderr=%s", code, out.String(), errs.String())
	}
	var got response
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Data["source"] != "specify" {
		t.Errorf("source = %v, want specify", got.Data["source"])
	}

	// docs/constitution.md wins over the Spec Kit location.
	repo.Write("docs/constitution.md", "# Hermoso constitution\n")
	out.Reset()
	errs.Reset()
	if code := Run(ctx, []string{"context", "constitution", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("context constitution (docs): code=%d output=%s", code, out.String())
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Data["source"] != "docs" {
		t.Errorf("source = %v, want docs", got.Data["source"])
	}

	// No constitution anywhere: a validation error names all candidates.
	empty := testutil.NewRepository(t)
	out.Reset()
	errs.Reset()
	if code := Run(ctx, []string{"context", "constitution", empty.Root, "--json"}, deps); code != ExitFailure {
		t.Fatalf("context constitution (none): code=%d, want failure", code)
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error == nil || got.Error.Code != ErrorValidation {
		t.Errorf("error = %#v, want validation", got.Error)
	}
	if !strings.Contains(out.String(), "constitution") {
		t.Errorf("error does not name the constitution candidates: %q", out.String())
	}
}

func TestContextConstitutionProjectOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	out, errs := &bytes.Buffer{}, &bytes.Buffer{}
	deps := testDependencies(out, errs)
	repo := testutil.NewRepository(t)
	repo.Write("docs/constitution.md", "# default\n")
	repo.Write("GOVERNANCE.md", "# override\n")
	// Set the override via project.json (the field is optional; the command
	// reads it best-effort and works on uninitialized repositories too).
	repo.Write(".hermoso/project.json",
		`{"schema_version":"2","project_id":"project-override","target":{"repository":"`+repo.Root+`"},`+"\n"+
		` "kanban_tenant":"hermoso-project-override","created_at":"2026-01-01T00:00:00Z","constitution_path":"GOVERNANCE.md"}`+"\n")

	out.Reset()
	errs.Reset()
	if code := Run(ctx, []string{"context", "constitution", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("context constitution (override): code=%d output=%s stderr=%s", code, out.String(), errs.String())
	}
	var got response
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Data["source"] != "project_override" {
		t.Errorf("source = %v, want project_override", got.Data["source"])
	}
}
