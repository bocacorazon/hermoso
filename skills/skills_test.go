package skills_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var skillNames = []string{"hermoso", "hermoso-design", "hermoso-construction"}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSkillFrontmatterAndCrossReferences(t *testing.T) {
	root := repositoryRoot(t)
	nameRE := regexp.MustCompile(`(?m)^name: ([a-z0-9-]+)$`)
	descriptionRE := regexp.MustCompile(`(?m)^description: "Use when .+"$`)
	relatedRE := regexp.MustCompile(`(?m)^    related_skills: \[([^\]]*)\]$`)
	known := map[string]bool{}
	for _, name := range skillNames {
		known[name] = true
	}

	for _, name := range skillNames {
		content := read(t, filepath.Join(root, "skills", name, "SKILL.md"))
		if !strings.HasPrefix(content, "---\n") {
			t.Errorf("%s: frontmatter must start at byte zero", name)
		}
		if strings.Count(content, "\n---\n") < 1 {
			t.Errorf("%s: missing frontmatter terminator", name)
		}
		match := nameRE.FindStringSubmatch(content)
		if len(match) != 2 || match[1] != name {
			t.Errorf("%s: frontmatter name mismatch", name)
		}
		if !descriptionRE.MatchString(content) {
			t.Errorf("%s: description must start with Use when", name)
		}
		related := relatedRE.FindStringSubmatch(content)
		if len(related) != 2 {
			t.Errorf("%s: missing related_skills", name)
			continue
		}
		for _, ref := range strings.Split(related[1], ",") {
			ref = strings.TrimSpace(ref)
			if !known[ref] {
				t.Errorf("%s: unresolved related skill %q", name, ref)
			}
		}
	}
}

func TestDefaultProfileBindings(t *testing.T) {
	root := repositoryRoot(t)
	var profile struct {
		SchemaVersion string         `json:"schema_version"`
		Phases        map[string]any `json:"phases"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(root, "profiles", "default.yaml"))), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.SchemaVersion != "hermoso-profile/v1" {
		t.Fatalf("schema version = %q", profile.SchemaVersion)
	}
	for _, phase := range []string{"controller", "design", "construction", "integration"} {
		if profile.Phases[phase] == nil {
			t.Errorf("missing phase %q", phase)
		}
	}
	content := read(t, filepath.Join(root, "profiles", "default.yaml"))
	for _, skill := range []string{
		"kanban-orchestrator", "test-driven-development",
		"systematic-debugging", "spike", "hermoso-design", "hermoso-construction",
	} {
		if !strings.Contains(content, `"`+skill+`"`) {
			t.Errorf("profile missing skill %q", skill)
		}
	}
}

func TestSkillsDoNotInstructDirectStateMutationOrInstallation(t *testing.T) {
	root := repositoryRoot(t)
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`(?im)^\s*(sed|tee|cp|install)\b[^\n]*\.hermoso/`),
		regexp.MustCompile(`(?im)^\s*(cp|install)\b[^\n]*(skill|SKILL\.md)[^\n]*target repositor`),
	}
	for _, name := range skillNames {
		content := read(t, filepath.Join(root, "skills", name, "SKILL.md"))
		lower := strings.ToLower(content)
		if !strings.Contains(lower, ".hermoso") ||
			(!strings.Contains(lower, "never edit") &&
				!strings.Contains(lower, "do not edit") &&
				!strings.Contains(lower, "mutate hermoso state")) {
			t.Errorf("%s must explicitly prohibit direct state mutation", name)
		}
		for _, pattern := range forbidden {
			if pattern.MatchString(content) {
				t.Errorf("%s contains forbidden instruction matching %s", name, pattern)
			}
		}
	}
}

func TestCommandLabelsMatchCurrentCLI(t *testing.T) {
	root := repositoryRoot(t)
	all := ""
	for _, name := range skillNames {
		all += read(t, filepath.Join(root, "skills", name, "SKILL.md"))
	}
	for _, current := range []string{
		"hermoso status", "hermoso schema feature-design",
		"hermoso context",
		"hermoso schema work-graph", "hermoso schema phase-result",
		"hermoso validate feature-design", "hermoso validate work-graph",
		"hermoso validate phase-result",
		"hermoso design put", "hermoso approve design", "hermoso graph put",
		"hermoso construction prepare", "hermoso construction ready",
		"hermoso task bind", "hermoso work start", "hermoso work complete",
		"hermoso work block", "hermoso construction integrate",
		"hermoso result put", "hermoso resume",
	} {
		if !strings.Contains(all, current) {
			t.Errorf("missing current command %q", current)
		}
	}
}

func TestSkillsRequireExplicitContextChecks(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range skillNames {
		content := strings.ToLower(read(t, filepath.Join(root, "skills", name, "SKILL.md")))
		for _, required := range []string{"hermoso context", "absolute workspace", "block", "mismatch"} {
			if !strings.Contains(content, required) {
				t.Errorf("%s must require %q at boundaries", name, required)
			}
		}
		if !strings.Contains(content, "never infer") {
			t.Errorf("%s must prohibit inferred identity", name)
		}
	}
}
