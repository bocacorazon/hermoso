package domain

import (
	"encoding/json"
	"testing"
	"time"
)

var domainTestTime = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

func TestProjectProfilePathRoundTrips(t *testing.T) {
	t.Parallel()

	original := Project{
		SchemaVersion: SchemaVersion,
		ProjectID:     "project-test",
		Target: TargetIdentity{
			Repository: "/repos/test",
		},
		KanbanTenant: KanbanTenant("project-test"),
		CreatedAt:    domainTestTime,
		ProfilePath:  "/repos/profiles/my-profile.json",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored Project
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.ProfilePath != original.ProfilePath {
		t.Errorf("ProfilePath = %q, want %q", restored.ProfilePath, original.ProfilePath)
	}
}

func TestProjectProfilePathOmitsWhenEmpty(t *testing.T) {
	t.Parallel()

	project := Project{
		SchemaVersion: SchemaVersion,
		ProjectID:     "project-test",
		Target: TargetIdentity{
			Repository: "/repos/test",
		},
		KanbanTenant: KanbanTenant("project-test"),
		CreatedAt:    domainTestTime,
	}

	data, err := json.Marshal(project)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if _, present := raw["profile_path"]; present {
		t.Errorf("profile_path should be omitted when empty, got: %s", data)
	}
}

func TestRunProfilePathRoundTrips(t *testing.T) {
	t.Parallel()

	original := Run{
		SchemaVersion: SchemaVersion,
		Context: ContextRef{
			SchemaVersion: ContextSchemaVersion,
			ProjectID:     "project-test",
			FeatureID:     "feature-test",
			RunID:         "run-test",
			Repository:    TargetIdentity{Repository: "/repos/test"},
		},
		Phase:       PhaseDesign,
		Status:      StatusPending,
		Revision:    1,
		CreatedAt:   domainTestTime,
		UpdatedAt:   domainTestTime,
		ProfilePath: "/repos/profiles/run-override.json",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored Run
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.ProfilePath != original.ProfilePath {
		t.Errorf("ProfilePath = %q, want %q", restored.ProfilePath, original.ProfilePath)
	}
}

func TestRunProfilePathOmitsWhenEmpty(t *testing.T) {
	t.Parallel()

	run := Run{
		SchemaVersion: SchemaVersion,
		Context: ContextRef{
			SchemaVersion: ContextSchemaVersion,
			ProjectID:     "project-test",
			FeatureID:     "feature-test",
			RunID:         "run-test",
			Repository:    TargetIdentity{Repository: "/repos/test"},
		},
		Phase:     PhaseDesign,
		Status:    StatusPending,
		Revision:  1,
		CreatedAt: domainTestTime,
		UpdatedAt: domainTestTime,
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if _, present := raw["profile_path"]; present {
		t.Errorf("profile_path should be omitted when empty, got: %s", data)
	}
}
