package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bocacorazon/hermoso/internal/domain"
)

type QueryMode string

const (
	QueryOrientation QueryMode = "orientation"
	QueryTask        QueryMode = "task"
	QueryImpact      QueryMode = "impact"
	QueryEvidence    QueryMode = "evidence"
)

type QueryRequest struct {
	Mode        QueryMode
	Text        string
	NodeIDs     []string
	BudgetBytes int
}

type QueryResult struct {
	SchemaVersion  string             `json:"schema_version"`
	SnapshotID     string             `json:"snapshot_id"`
	ContentHash    string             `json:"content_hash"`
	SourceRevision string             `json:"source_revision"`
	Mode           QueryMode          `json:"mode"`
	Query          string             `json:"query,omitempty"`
	Nodes          []domain.ModelNode `json:"nodes"`
	Edges          []domain.ModelEdge `json:"edges"`
	Truncated      bool               `json:"truncated"`
	Rendered       string             `json:"rendered"`
}

type ModelStatus struct {
	Exists       bool                 `json:"exists"`
	Pointer      *domain.ModelPointer `json:"pointer,omitempty"`
	HeadRevision string               `json:"head_revision"`
	Fresh        bool                 `json:"fresh"`
	StaleReasons []string             `json:"stale_reasons,omitempty"`
}

func Query(snapshot domain.ModelSnapshot, request QueryRequest) (QueryResult, error) {
	if err := snapshot.Validate(); err != nil {
		return QueryResult{}, fmt.Errorf("invalid model snapshot: %w", err)
	}
	if request.BudgetBytes == 0 {
		request.BudgetBytes = 12 * 1024
	}
	if request.BudgetBytes < 1024 {
		return QueryResult{}, errors.New("query budget must be at least 1024 bytes")
	}
	if request.Mode != QueryOrientation && request.Mode != QueryTask &&
		request.Mode != QueryImpact && request.Mode != QueryEvidence {
		return QueryResult{}, fmt.Errorf("unknown query mode %q", request.Mode)
	}
	if request.Mode == QueryTask && strings.TrimSpace(request.Text) == "" {
		return QueryResult{}, errors.New("task query requires search text")
	}
	if (request.Mode == QueryImpact || request.Mode == QueryEvidence) && len(request.NodeIDs) == 0 {
		return QueryResult{}, fmt.Errorf("%s query requires at least one node ID", request.Mode)
	}

	nodeByID := make(map[string]domain.ModelNode, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		nodeByID[node.ID] = node
	}
	edgeByNode := map[string][]domain.ModelEdge{}
	for _, edge := range snapshot.Edges {
		edgeByNode[edge.Source] = append(edgeByNode[edge.Source], edge)
		edgeByNode[edge.Target] = append(edgeByNode[edge.Target], edge)
	}

	var candidates []scoredNode
	switch request.Mode {
	case QueryOrientation:
		for _, node := range snapshot.Nodes {
			score := orientationScore(node)
			if request.Text != "" {
				score += textScore(node, tokenize(request.Text))
			}
			if score > 0 {
				candidates = append(candidates, scoredNode{node: node, score: score})
			}
		}
	case QueryTask:
		terms := tokenize(request.Text)
		for _, node := range snapshot.Nodes {
			if score := textScore(node, terms); score > 0 {
				candidates = append(candidates, scoredNode{node: node, score: score})
			}
		}
	case QueryImpact:
		seen := map[string]int{}
		queue := append([]string(nil), request.NodeIDs...)
		for _, id := range request.NodeIDs {
			if _, ok := nodeByID[id]; !ok {
				return QueryResult{}, fmt.Errorf("unknown model node %q", id)
			}
			seen[id] = 100
		}
		for len(queue) != 0 {
			id := queue[0]
			queue = queue[1:]
			score := seen[id]
			if score <= 10 {
				continue
			}
			for _, edge := range edgeByNode[id] {
				next := edge.Source
				if next == id {
					next = edge.Target
				}
				if _, ok := seen[next]; !ok {
					seen[next] = score - 10
					queue = append(queue, next)
				}
			}
		}
		for id, score := range seen {
			candidates = append(candidates, scoredNode{node: nodeByID[id], score: score})
		}
	case QueryEvidence:
		for _, id := range request.NodeIDs {
			node, ok := nodeByID[id]
			if !ok {
				return QueryResult{}, fmt.Errorf("unknown model node %q", id)
			}
			candidates = append(candidates, scoredNode{node: node, score: 100})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].node.ID < candidates[j].node.ID
	})

	selected, selectedIDs, truncated := selectWithinBudget(candidates, request.BudgetBytes)
	var edges []domain.ModelEdge
	for _, edge := range snapshot.Edges {
		_, source := selectedIDs[edge.Source]
		_, target := selectedIDs[edge.Target]
		if source && target {
			edges = append(edges, edge)
		}
	}
	rendered := renderQuery(request.Mode, snapshot.Manifest, selected, edges)
	return QueryResult{
		SchemaVersion: domain.ModelSchemaVersion, SnapshotID: snapshot.Manifest.SnapshotID,
		ContentHash: snapshot.Manifest.ContentHash, SourceRevision: snapshot.Manifest.SourceRevision,
		Mode: request.Mode, Query: request.Text, Nodes: selected, Edges: edges,
		Truncated: truncated, Rendered: rendered,
	}, nil
}

type scoredNode struct {
	node  domain.ModelNode
	score int
}

func selectWithinBudget(candidates []scoredNode, budget int) ([]domain.ModelNode, map[string]struct{}, bool) {
	selected := make([]domain.ModelNode, 0, len(candidates))
	ids := map[string]struct{}{}
	used := 0
	truncated := false
	for _, candidate := range candidates {
		data, _ := json.Marshal(candidate.node)
		cost := len(data)
		if used+cost > budget {
			truncated = true
			break
		}
		selected = append(selected, candidate.node)
		ids[candidate.node.ID] = struct{}{}
		used += cost
	}
	return selected, ids, truncated
}

func orientationScore(node domain.ModelNode) int {
	switch node.Kind {
	case "repository":
		return 100
	case "domain":
		return 90
	case "component":
		return 80
	case "interface":
		return 75
	case "invariant":
		return 70
	case "command":
		return 60
	case "test":
		return 50
	case "decision", "concept":
		return 45
	default:
		return 0
	}
}

func textScore(node domain.ModelNode, terms []string) int {
	haystacks := []struct {
		value  string
		weight int
	}{
		{node.ID, 8}, {node.Title, 10}, {node.Summary, 6}, {node.Kind, 4},
	}
	for key, value := range node.Attributes {
		haystacks = append(haystacks, struct {
			value  string
			weight int
		}{key + " " + value, 5})
	}
	for _, evidence := range node.Evidence {
		haystacks = append(haystacks, struct {
			value  string
			weight int
		}{evidence.Path, 7})
	}
	score := 0
	for _, term := range terms {
		matched := false
		for _, haystack := range haystacks {
			if strings.Contains(strings.ToLower(haystack.value), term) {
				score += haystack.weight
				matched = true
			}
		}
		if !matched {
			return 0
		}
	}
	return score
}

func tokenize(value string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, term := range strings.Fields(strings.ToLower(value)) {
		term = strings.Trim(term, ".,:;()[]{}\"'")
		if len(term) < 2 {
			continue
		}
		if _, ok := seen[term]; !ok {
			result = append(result, term)
			seen[term] = struct{}{}
		}
	}
	return result
}

func renderQuery(mode QueryMode, manifest domain.ModelManifest, nodes []domain.ModelNode, edges []domain.ModelEdge) string {
	var output strings.Builder
	fmt.Fprintf(&output, "# Model %s view\n\nSnapshot: `%s`\n\nSource revision: `%s`\n\n", mode, manifest.SnapshotID, manifest.SourceRevision)
	for _, node := range nodes {
		fmt.Fprintf(&output, "- `%s` — **%s**: %s", node.ID, node.Title, node.Summary)
		if len(node.Evidence) != 0 {
			fmt.Fprintf(&output, " (`%s`", node.Evidence[0].Path)
			if node.Evidence[0].StartLine != 0 {
				fmt.Fprintf(&output, ":%d", node.Evidence[0].StartLine)
			}
			output.WriteString(")")
		}
		output.WriteByte('\n')
	}
	if len(edges) != 0 {
		output.WriteString("\n## Relationships\n\n")
		for _, edge := range edges {
			fmt.Fprintf(&output, "- `%s` —%s→ `%s`\n", edge.Source, edge.Relation, edge.Target)
		}
	}
	return output.String()
}
