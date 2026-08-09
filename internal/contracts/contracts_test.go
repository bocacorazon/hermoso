package contracts

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func TestSchemasAreDeterministicJSONSchemas(t *testing.T) {
	t.Parallel()

	for _, kind := range []Kind{FeatureDesign, WorkGraph, PhaseResult} {
		kind := kind
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			first, err := Schema(kind)
			if err != nil {
				t.Fatal(err)
			}
			second, err := Schema(kind)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Fatal("schema output is not deterministic")
			}
			var decoded map[string]any
			if err := json.Unmarshal(first, &decoded); err != nil {
				t.Fatalf("schema is not JSON: %v", err)
			}
			if decoded["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
				t.Errorf("$schema = %v", decoded["$schema"])
			}
			if decoded["additionalProperties"] != false {
				t.Error("root schema must reject unknown properties")
			}
			assertLocalRefsResolve(t, decoded, decoded)
		})
	}
}

func assertLocalRefsResolve(t *testing.T, root map[string]any, value any) {
	t.Helper()
	switch current := value.(type) {
	case map[string]any:
		if reference, ok := current["$ref"].(string); ok {
			const prefix = "#/$defs/"
			if !strings.HasPrefix(reference, prefix) {
				t.Fatalf("unsupported schema reference %q", reference)
			}
			definitions, ok := root["$defs"].(map[string]any)
			if !ok {
				t.Fatal("schema has no $defs object")
			}
			if _, ok := definitions[strings.TrimPrefix(reference, prefix)]; !ok {
				t.Fatalf("unresolved schema reference %q", reference)
			}
		}
		for _, child := range current {
			assertLocalRefsResolve(t, root, child)
		}
	case []any:
		for _, child := range current {
			assertLocalRefsResolve(t, root, child)
		}
	}
}

func TestValidateFixtures(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		kind Kind
		file string
	}{
		{FeatureDesign, "feature-design.valid.json"},
		{WorkGraph, "work-graph.valid.json"},
		{PhaseResult, "phase-result.valid.json"},
	} {
		data, err := os.ReadFile(filepath.Join("testdata", test.file))
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(test.kind, data); err != nil {
			t.Errorf("%s: %v", test.file, err)
		}
	}
}

func TestValidateRejectsSemanticAndStructuralErrors(t *testing.T) {
	t.Parallel()

	cyclic, err := os.ReadFile(filepath.Join("testdata", "work-graph.invalid.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(WorkGraph, cyclic); err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("cycle error = %v", err)
	}

	unknown := bytes.Replace(cyclic, []byte(`"revision": 1,`), []byte(`"revision": 1, "surprise": true,`), 1)
	if err := Validate(WorkGraph, unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field error = %v", err)
	}

	if err := Validate(FeatureDesign, append([]byte(`{}`), []byte(` {}`)...)); err == nil {
		t.Fatal("multiple JSON values accepted")
	}
}

func TestParseKind(t *testing.T) {
	t.Parallel()
	if _, err := ParseKind("project"); err == nil {
		t.Fatal("unsupported kind accepted")
	}
}

func TestValidateForContextRejectsMismatch(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "work-graph.valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	expected := domain.ContextRef{
		SchemaVersion: domain.ContextSchemaVersion,
		ProjectID:     "project-1",
		FeatureID:     "other-feature",
		RunID:         "run-1",
		Repository: domain.TargetIdentity{
			Repository:    "/workspace/project",
			RemoteURL:     "https://github.com/example/project.git",
			DefaultBranch: "main",
		},
	}
	if err := ValidateForContext(WorkGraph, data, expected); err == nil || !strings.Contains(err.Error(), "context does not match") {
		t.Fatalf("mismatch error = %v", err)
	}
}
