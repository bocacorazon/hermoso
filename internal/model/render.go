package model

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func renderViews(project domain.Project, revision string, nodes []domain.ModelNode, edges []domain.ModelEdge) map[string]string {
	byKind := map[string][]domain.ModelNode{}
	for _, node := range nodes {
		byKind[node.Kind] = append(byKind[node.Kind], node)
	}
	for kind := range byKind {
		sort.Slice(byKind[kind], func(i, j int) bool { return byKind[kind][i].ID < byKind[kind][j].ID })
	}
	views := map[string]string{}
	views["index.md"] = fmt.Sprintf(
		"# Repository knowledge spine\n\nProject: `%s`\n\nSource revision: `%s`\n\nNodes: %d\n\nEdges: %d\n",
		project.ProjectID, revision, len(nodes), len(edges),
	)
	views["views/system-context.md"] = renderNodeList("System context", append(append([]domain.ModelNode{}, byKind["repository"]...), byKind["interface"]...))
	views["views/domains.md"] = renderNodeList("Domains", byKind["domain"])
	views["views/components.md"] = renderNodeList("Components", byKind["component"])
	views["views/runtime.md"] = renderNodeList("Runtime", append(append([]domain.ModelNode{}, byKind["interface"]...), byKind["command"]...))
	views["views/data.md"] = renderAspectList("Data", nodes, "data")
	views["views/interfaces.md"] = renderInterfaces(byKind["interface"])
	views["views/testing.md"] = renderTesting(byKind["test"], edges)
	views["views/operations.md"] = renderNodeList("Operations", byKind["command"])
	views["views/decisions.md"] = renderNodeList("Decisions", byKind["decision"])
	views["views/vocabulary.md"] = renderNodeList("Vocabulary", append(append([]domain.ModelNode{}, byKind["concept"]...), byKind["invariant"]...))
	return views
}

func renderNodeList(title string, nodes []domain.ModelNode) string {
	var output strings.Builder
	fmt.Fprintf(&output, "# %s\n\n", title)
	if len(nodes) == 0 {
		output.WriteString("No indexed entries.\n")
		return output.String()
	}
	for _, node := range nodes {
		fmt.Fprintf(&output, "- `%s` — **%s**: %s\n", node.ID, node.Title, node.Summary)
	}
	return output.String()
}

func renderAspectList(title string, nodes []domain.ModelNode, aspect string) string {
	var selected []domain.ModelNode
	for _, node := range nodes {
		for _, candidate := range node.Aspects {
			if candidate == aspect {
				selected = append(selected, node)
				break
			}
		}
	}
	return renderNodeList(title, selected)
}

func renderInterfaces(nodes []domain.ModelNode) string {
	var output strings.Builder
	output.WriteString("# Interfaces\n\n")
	if len(nodes) == 0 {
		output.WriteString("No indexed entries.\n")
		return output.String()
	}
	output.WriteString("| ID | Kind | Title | Evidence |\n| --- | --- | --- | --- |\n")
	for _, node := range nodes {
		evidence := ""
		if len(node.Evidence) != 0 {
			evidence = node.Evidence[0].Path
		}
		fmt.Fprintf(&output, "| `%s` | %s | %s | `%s` |\n", node.ID, node.Attributes["interface_kind"], node.Title, evidence)
	}
	return output.String()
}

func renderTesting(tests []domain.ModelNode, edges []domain.ModelEdge) string {
	var output strings.Builder
	output.WriteString("# Testing\n\n")
	if len(tests) == 0 {
		output.WriteString("No indexed test surfaces.\n")
	} else {
		for _, test := range tests {
			fmt.Fprintf(&output, "- `%s` — %s\n", test.ID, test.Title)
		}
	}
	output.WriteString("\n## Exercises\n\n")
	count := 0
	for _, edge := range edges {
		if edge.Relation == "exercises" {
			fmt.Fprintf(&output, "- `%s` → `%s`\n", edge.Source, edge.Target)
			count++
		}
	}
	if count == 0 {
		output.WriteString("No test-to-source relationships inferred.\n")
	}
	return output.String()
}
