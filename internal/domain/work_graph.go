package domain

import (
	"fmt"
	"strings"
)

type SkillBinding struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type Worker struct {
	Profile string         `json:"profile"`
	Skills  []SkillBinding `json:"skills"`
}

type WorkItem struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Prompt               string   `json:"prompt"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
	Parents              []string `json:"parents,omitempty"`
	Worker               Worker   `json:"worker"`
	ChangedSurfaces      []string `json:"expected_changed_surfaces,omitempty"`
	ValidationCommands   []string `json:"validation_commands,omitempty"`
	RuntimeBudgetSeconds uint64   `json:"runtime_budget_seconds,omitempty"`
}

type WorkGraph struct {
	SchemaVersion string     `json:"schema_version"`
	Context       ContextRef `json:"context"`
	Producer      Producer   `json:"producer"`
	Revision      uint64     `json:"revision"`
	Items         []WorkItem `json:"items"`
}

func (g WorkGraph) Validate() error {
	var errs ValidationErrors
	validateVersion(g.SchemaVersion, &errs)
	g.Context.validate("context", &errs)
	g.Producer.validate("producer", &errs)
	if g.Revision == 0 {
		errs.add("revision", "must be greater than zero")
	}
	if len(g.Items) == 0 {
		errs.add("items", "must contain at least one work item")
	}
	ids := make(map[string]int, len(g.Items))
	for i, item := range g.Items {
		path := fmt.Sprintf("items[%d]", i)
		validateID(path+".id", item.ID, &errs)
		if prior, ok := ids[item.ID]; ok {
			errs.add(path+".id", fmt.Sprintf("duplicates items[%d].id", prior))
		} else {
			ids[item.ID] = i
		}
		validateRequired(path+".title", item.Title, &errs)
		validateRequired(path+".prompt", item.Prompt, &errs)
		validateStringList(path+".acceptance_criteria", item.AcceptanceCriteria, true, &errs)
		validateWorker(path+".worker", item.Worker, &errs)
		validateStringList(path+".expected_changed_surfaces", item.ChangedSurfaces, false, &errs)
		validateStringList(path+".validation_commands", item.ValidationCommands, false, &errs)
	}
	for i, item := range g.Items {
		seenParents := make(map[string]struct{}, len(item.Parents))
		for j, parent := range item.Parents {
			path := fmt.Sprintf("items[%d].parents[%d]", i, j)
			if parent == item.ID {
				errs.add(path, "must not reference the work item itself")
			}
			if _, ok := ids[parent]; !ok {
				errs.add(path, "references an unknown work item")
			}
			if _, ok := seenParents[parent]; ok {
				errs.add(path, "duplicates an earlier parent")
			}
			seenParents[parent] = struct{}{}
		}
	}
	if cycle := findCycle(g.Items, ids); len(cycle) != 0 {
		errs.add("items", "dependency cycle: "+strings.Join(cycle, " -> "))
	}
	return validationResult(errs)
}

func validateWorker(path string, worker Worker, errs *ValidationErrors) {
	if !profilePattern.MatchString(worker.Profile) {
		errs.add(path+".profile", "must be a valid non-empty profile name")
	}
	if len(worker.Skills) == 0 {
		errs.add(path+".skills", "must contain at least one ordered skill binding")
	}
	seen := make(map[string]struct{}, len(worker.Skills))
	for i, skill := range worker.Skills {
		skillPath := fmt.Sprintf("%s.skills[%d]", path, i)
		if !profilePattern.MatchString(skill.Name) {
			errs.add(skillPath+".name", "must be a valid non-empty skill name")
		}
		if _, ok := seen[skill.Name]; ok {
			errs.add(skillPath+".name", "must not duplicate an earlier skill")
		}
		seen[skill.Name] = struct{}{}
	}
}

func findCycle(items []WorkItem, ids map[string]int) []string {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(items))
	stack := make([]string, 0, len(items))
	var visit func(string) []string
	visit = func(id string) []string {
		state[id] = visiting
		stack = append(stack, id)
		for _, parent := range items[ids[id]].Parents {
			if _, exists := ids[parent]; !exists {
				continue
			}
			if state[parent] == visiting {
				start := 0
				for stack[start] != parent {
					start++
				}
				return append(append([]string(nil), stack[start:]...), parent)
			}
			if state[parent] == unvisited {
				if cycle := visit(parent); len(cycle) != 0 {
					return cycle
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = visited
		return nil
	}
	for _, item := range items {
		if state[item.ID] == unvisited {
			if cycle := visit(item.ID); len(cycle) != 0 {
				return cycle
			}
		}
	}
	return nil
}
