package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/domain"
)

const (
	gitExtractorVersion       = "git/v1"
	structureExtractorVersion = "tree-sitter/v1"
	manifestExtractorVersion  = "manifests/v1"
	markdownExtractorVersion  = "markdown/v1"
	maxParsedFileBytes        = int64(2 * 1024 * 1024)
)

type BuildRequest struct {
	Project        domain.Project
	RepositoryRoot string
	Revision       string
	SCIPPath       string
	GeneratedAt    time.Time
	Previous       *domain.ModelSnapshot
}

type FileFact struct {
	Path        string
	GitObject   string
	Size        int64
	ContentHash string
	Content     []byte
	Language    string
	Test        bool
}

type graph struct {
	nodes map[string]domain.ModelNode
	edges map[string]domain.ModelEdge
}

func ResolveRevision(ctx context.Context, repositoryRoot, revision string) (string, error) {
	resolved, err := gitOutput(ctx, repositoryRoot, "rev-parse", "--verify", revision+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve source revision: %w", err)
	}
	return resolved, nil
}

func Build(ctx context.Context, request BuildRequest) (domain.ModelSnapshot, error) {
	if err := request.Project.Validate(); err != nil {
		return domain.ModelSnapshot{}, fmt.Errorf("invalid project: %w", err)
	}
	root, err := filepath.EvalSymlinks(request.RepositoryRoot)
	if err != nil {
		return domain.ModelSnapshot{}, fmt.Errorf("resolve repository root: %w", err)
	}
	if root != request.Project.Target.Repository {
		return domain.ModelSnapshot{}, errors.New("repository root does not match project identity")
	}
	if request.GeneratedAt.IsZero() {
		return domain.ModelSnapshot{}, errors.New("generated time must be set")
	}
	revision, err := gitOutput(ctx, root, "rev-parse", "--verify", request.Revision+"^{commit}")
	if err != nil {
		return domain.ModelSnapshot{}, fmt.Errorf("resolve source revision: %w", err)
	}

	files, err := inventory(ctx, root, revision)
	if err != nil {
		return domain.ModelSnapshot{}, err
	}
	extractors := []domain.ModelExtractor{
		{Name: "git", Version: gitExtractorVersion},
		{Name: "tree-sitter", Version: structureExtractorVersion},
		{Name: "manifests", Version: manifestExtractorVersion},
		{Name: "markdown", Version: markdownExtractorVersion},
	}
	scipData, scipExtractor, scipInput, err := loadSCIP(request.SCIPPath)
	if err != nil {
		return domain.ModelSnapshot{}, err
	}
	var inputs []domain.ModelInput
	if scipExtractor != nil {
		extractors = append(extractors, *scipExtractor)
		inputs = append(inputs, *scipInput)
	}

	result := &graph{nodes: map[string]domain.ModelNode{}, edges: map[string]domain.ModelEdge{}}
	addFileNodes(result, files)
	reused := reusablePaths(request.Previous, files, extractors)
	scipReusable := request.Previous == nil || sameInputs(request.Previous.Manifest.Inputs, inputs)
	if len(reused) != 0 {
		reuseUnchangedNodes(result, *request.Previous, reused, scipReusable)
	}
	for _, file := range files {
		if reused[file.Path] {
			continue
		}
		if file.Language != "" && len(file.Content) != 0 {
			if err := extractStructure(ctx, result, file, files); err != nil {
				return domain.ModelSnapshot{}, fmt.Errorf("extract %s: %w", file.Path, err)
			}
		}
		classifyFileInterface(result, file)
		extractManifestCommands(result, file)
		extractMarkdownKnowledge(result, file)
	}
	addComponents(result, files)
	addTestRelationships(result, files)
	addRepositoryNode(result, request.Project, files)
	if scipData != nil {
		if err := augmentSCIP(result, scipData, files); err != nil {
			return domain.ModelSnapshot{}, err
		}
	}
	if request.Previous != nil && len(reused) != 0 {
		reuseUnchangedEdges(result, *request.Previous, scipReusable)
	}

	nodes := mapNodes(result.nodes)
	edges := mapEdges(result.edges)
	snapshot := domain.ModelSnapshot{
		Manifest: domain.ModelManifest{
			SchemaVersion:     domain.ModelSchemaVersion,
			ProjectID:         request.Project.ProjectID,
			Repository:        request.Project.Target,
			SourceRevision:    revision,
			VocabularyVersion: "v1",
			GeneratedAt:       request.GeneratedAt.UTC(),
			Extractors:        extractors,
			Inputs:            inputs,
		},
		Vocabulary: domain.DefaultModelVocabulary(),
		Nodes:      nodes,
		Edges:      edges,
		Views:      renderViews(request.Project, revision, nodes, edges),
	}
	return domain.FinalizeModelSnapshot(snapshot)
}

func inventory(ctx context.Context, root, revision string) ([]FileFact, error) {
	output, err := gitBytes(ctx, root, "ls-tree", "-r", "-l", "-z", "--full-tree", revision)
	if err != nil {
		return nil, fmt.Errorf("list repository tree: %w", err)
	}
	entries := bytes.Split(output, []byte{0})
	files := make([]FileFact, 0, len(entries))
	for _, entry := range entries {
		if len(entry) == 0 {
			continue
		}
		tab := bytes.IndexByte(entry, '\t')
		if tab < 0 {
			return nil, errors.New("parse Git tree: missing path separator")
		}
		fields := strings.Fields(string(entry[:tab]))
		if len(fields) != 4 || fields[1] != "blob" {
			return nil, fmt.Errorf("parse Git tree entry %q", entry)
		}
		filePath := string(entry[tab+1:])
		if !safeRepositoryPath(filePath) {
			return nil, fmt.Errorf("Git tree contains unsafe path %q", filePath)
		}
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse size for %s: %w", filePath, err)
		}
		fact := FileFact{
			Path: filePath, GitObject: fields[2], Size: size,
			Language: languageForPath(filePath), Test: isTestPath(filePath),
		}
		if size <= maxParsedFileBytes && shouldRead(filePath) {
			fact.Content, err = gitBytes(ctx, root, "cat-file", "blob", fields[2])
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", filePath, err)
			}
			fact.ContentHash = digest.Bytes(fact.Content)
		} else {
			fact.ContentHash, err = gitBlobHash(ctx, root, fields[2])
			if err != nil {
				return nil, fmt.Errorf("hash %s: %w", filePath, err)
			}
		}
		files = append(files, fact)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func addFileNodes(result *graph, files []FileFact) {
	producer := domain.ModelProducer{Kind: "extractor", Name: "git", Version: gitExtractorVersion}
	for _, file := range files {
		aspects := []string{"structure"}
		if file.Test {
			aspects = append(aspects, "testing")
		}
		attributes := map[string]string{
			"git_object": file.GitObject,
			"size_bytes": strconv.FormatInt(file.Size, 10),
		}
		if file.Language != "" {
			attributes["language"] = file.Language
		}
		result.addNode(domain.ModelNode{
			ID: "file:" + file.Path, Kind: "file", Abstraction: "code", Aspects: aspects,
			Title: path.Base(file.Path), Summary: "Tracked repository file.",
			EpistemicStatus: "observed",
			Evidence:        []domain.ModelEvidence{{Path: file.Path, ContentHash: file.ContentHash}},
			Producer:        producer, Attributes: attributes,
		})
		if file.Test {
			result.addNode(domain.ModelNode{
				ID: "test:" + file.Path, Kind: "test", Abstraction: "code",
				Aspects: []string{"testing"}, Title: path.Base(file.Path),
				Summary: "Repository test surface.", EpistemicStatus: "observed",
				Evidence: []domain.ModelEvidence{{Path: file.Path, ContentHash: file.ContentHash}},
				Producer: producer, Attributes: map[string]string{"language": file.Language},
			})
			result.addEdge(edge("file:"+file.Path, "contains", "test:"+file.Path, "observed", producer, "file:"+file.Path))
		}
	}
}

func addComponents(result *graph, files []FileFact) {
	producer := domain.ModelProducer{Kind: "algorithm", Name: "directory-components", Version: "v1"}
	members := map[string][]string{}
	for _, file := range files {
		dir := path.Dir(file.Path)
		if dir == "." {
			dir = "root"
		} else {
			dir = strings.Split(dir, "/")[0]
		}
		members[dir] = append(members[dir], "file:"+file.Path)
	}
	for name, fileIDs := range members {
		sort.Strings(fileIDs)
		componentID := "component:" + name
		result.addNode(domain.ModelNode{
			ID: componentID, Kind: "component", Abstraction: "component",
			Aspects: []string{"structure"}, Title: name,
			Summary:         "Repository component derived from tracked paths.",
			EpistemicStatus: "derived", Producer: producer, DerivedFrom: fileIDs,
		})
		for _, fileID := range fileIDs {
			result.addEdge(edge(componentID, "contains", fileID, "derived", producer, fileID))
		}
	}
}

func addRepositoryNode(result *graph, project domain.Project, files []FileFact) {
	derived := make([]string, 0, len(files))
	for _, file := range files {
		derived = append(derived, "file:"+file.Path)
	}
	result.addNode(domain.ModelNode{
		ID: "repository:" + project.ProjectID, Kind: "repository", Abstraction: "system",
		Aspects: []string{"structure"}, Title: path.Base(project.Target.Repository),
		Summary: "Canonical target repository.", EpistemicStatus: "derived",
		Producer:    domain.ModelProducer{Kind: "algorithm", Name: "repository-root", Version: "v1"},
		DerivedFrom: derived,
	})
}

func addTestRelationships(result *graph, files []FileFact) {
	paths := make(map[string]struct{}, len(files))
	for _, file := range files {
		paths[file.Path] = struct{}{}
	}
	producer := domain.ModelProducer{Kind: "algorithm", Name: "test-pairing", Version: "v1"}
	for _, file := range files {
		if !file.Test {
			continue
		}
		for _, source := range testSourceCandidates(file.Path) {
			if _, ok := paths[source]; ok {
				result.addEdge(edge("test:"+file.Path, "exercises", "file:"+source, "derived", producer, "file:"+file.Path))
				break
			}
		}
	}
}

func (g *graph) addNode(node domain.ModelNode) {
	if existing, ok := g.nodes[node.ID]; ok {
		existing.Evidence = appendUniqueEvidence(existing.Evidence, node.Evidence...)
		existing.DerivedFrom = appendUnique(existing.DerivedFrom, node.DerivedFrom...)
		g.nodes[node.ID] = existing
		return
	}
	g.nodes[node.ID] = node
}

func (g *graph) addEdge(candidate domain.ModelEdge) {
	if _, ok := g.edges[candidate.ID]; !ok {
		g.edges[candidate.ID] = candidate
	}
}

func edge(source, relation, target, status string, producer domain.ModelProducer, evidence ...string) domain.ModelEdge {
	return domain.ModelEdge{
		ID: stableID("edge", source, relation, target), Source: source, Relation: relation, Target: target,
		EpistemicStatus: status, Producer: producer, Evidence: appendUnique(nil, evidence...),
	}
}

func stableID(prefix string, parts ...string) string {
	hash := digest.Bytes([]byte(strings.Join(parts, "\x00")))
	return prefix + ":" + strings.TrimPrefix(hash, "sha256:")[:24]
}

func mapNodes(values map[string]domain.ModelNode) []domain.ModelNode {
	result := make([]domain.ModelNode, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func mapEdges(values map[string]domain.ModelEdge) []domain.ModelEdge {
	result := make([]domain.ModelEdge, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func reusablePaths(previous *domain.ModelSnapshot, files []FileFact, extractors []domain.ModelExtractor) map[string]bool {
	result := map[string]bool{}
	if previous == nil || !sameExtractors(previous.Manifest.Extractors, extractors) {
		return result
	}
	current := make(map[string]string, len(files))
	for _, file := range files {
		current[file.Path] = file.ContentHash
	}
	for _, node := range previous.Nodes {
		if node.Kind != "file" || len(node.Evidence) != 1 {
			continue
		}
		evidence := node.Evidence[0]
		if current[evidence.Path] == evidence.ContentHash {
			result[evidence.Path] = true
		}
	}
	return result
}

func reuseUnchangedNodes(result *graph, previous domain.ModelSnapshot, reusable map[string]bool, scipReusable bool) {
	available := map[string]struct{}{}
	for filePath := range reusable {
		available["file:"+filePath] = struct{}{}
	}
	remaining := append([]domain.ModelNode(nil), previous.Nodes...)
	for {
		added := false
		next := remaining[:0]
		for _, node := range remaining {
			if node.Kind == "file" {
				continue
			}
			if strings.HasPrefix(node.Producer.Name, "scip-") && !scipReusable {
				continue
			}
			reuse := len(node.Evidence) != 0
			for _, evidence := range node.Evidence {
				if !reusable[evidence.Path] {
					reuse = false
					break
				}
			}
			if len(node.Evidence) == 0 && len(node.DerivedFrom) != 0 {
				reuse = true
				for _, source := range node.DerivedFrom {
					if _, ok := available[source]; !ok {
						reuse = false
						break
					}
				}
			}
			if reuse {
				result.addNode(node)
				available[node.ID] = struct{}{}
				added = true
			} else {
				next = append(next, node)
			}
		}
		if !added {
			return
		}
		remaining = next
	}
}

func reuseUnchangedEdges(result *graph, previous domain.ModelSnapshot, scipReusable bool) {
	for _, candidate := range previous.Edges {
		if strings.HasPrefix(candidate.Producer.Name, "scip-") && !scipReusable {
			continue
		}
		if _, source := result.nodes[candidate.Source]; !source {
			continue
		}
		if _, target := result.nodes[candidate.Target]; !target {
			continue
		}
		evidencePresent := true
		for _, evidence := range candidate.Evidence {
			if _, ok := result.nodes[evidence]; !ok {
				evidencePresent = false
				break
			}
		}
		if evidencePresent {
			result.addEdge(candidate)
		}
	}
}

func sameInputs(left, right []domain.ModelInput) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]domain.ModelInput(nil), left...)
	rightCopy := append([]domain.ModelInput(nil), right...)
	sort.Slice(leftCopy, func(i, j int) bool { return leftCopy[i].Name < leftCopy[j].Name })
	sort.Slice(rightCopy, func(i, j int) bool { return rightCopy[i].Name < rightCopy[j].Name })
	for i := range leftCopy {
		if leftCopy[i] != rightCopy[i] {
			return false
		}
	}
	return true
}

func sameExtractors(left, right []domain.ModelExtractor) bool {
	if len(left) != len(right) {
		return false
	}
	normalize := func(values []domain.ModelExtractor) []domain.ModelExtractor {
		copy := append([]domain.ModelExtractor(nil), values...)
		sort.Slice(copy, func(i, j int) bool { return copy[i].Name < copy[j].Name })
		return copy
	}
	left, right = normalize(left), normalize(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		if _, ok := seen[value]; !ok {
			values = append(values, value)
			seen[value] = struct{}{}
		}
	}
	return values
}

func appendUniqueEvidence(values []domain.ModelEvidence, additions ...domain.ModelEvidence) []domain.ModelEvidence {
	seen := map[string]struct{}{}
	for _, value := range values {
		seen[fmt.Sprintf("%s:%d:%d:%s", value.Path, value.StartLine, value.EndLine, value.ContentHash)] = struct{}{}
	}
	for _, value := range additions {
		key := fmt.Sprintf("%s:%d:%d:%s", value.Path, value.StartLine, value.EndLine, value.ContentHash)
		if _, ok := seen[key]; !ok {
			values = append(values, value)
			seen[key] = struct{}{}
		}
	}
	return values
}

func extractManifestCommands(result *graph, file FileFact) {
	switch path.Base(file.Path) {
	case "go.mod":
		for _, command := range []struct {
			id, category, value string
		}{
			{"go-build", "build", "go build ./..."},
			{"go-test", "test", "go test ./..."},
			{"go-vet", "lint", "go vet ./..."},
		} {
			addCommand(result, file, command.id, command.category, command.value, "derived")
		}
	case "package.json":
		var manifest struct {
			Scripts map[string]string `json:"scripts"`
			Bin     json.RawMessage   `json:"bin"`
		}
		if len(file.Content) == 0 || json.Unmarshal(file.Content, &manifest) != nil {
			return
		}
		names := make([]string, 0, len(manifest.Scripts))
		for name := range manifest.Scripts {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			addCommand(result, file, "npm-"+name, commandCategory(name), "npm run "+name, "observed")
		}
		if len(manifest.Bin) != 0 && string(manifest.Bin) != "null" {
			addInterface(result, file, "package-bin", "cli", "Package CLI entry points")
		}
	case "Makefile":
		for _, line := range strings.Split(string(file.Content), "\n") {
			if strings.HasPrefix(line, "\t") || !strings.Contains(line, ":") {
				continue
			}
			name := strings.TrimSpace(strings.SplitN(line, ":", 2)[0])
			if name != "" && !strings.ContainsAny(name, " =$") {
				addCommand(result, file, "make-"+name, commandCategory(name), "make "+name, "observed")
			}
		}
	}
	if file.Path == "pyproject.toml" {
		inScripts := false
		for _, line := range strings.Split(string(file.Content), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") {
				inScripts = trimmed == "[project.scripts]"
				continue
			}
			if inScripts && strings.Contains(trimmed, "=") {
				name := strings.TrimSpace(strings.SplitN(trimmed, "=", 2)[0])
				name = strings.Trim(name, `"'`)
				if name != "" {
					addCommand(result, file, "python-"+name, "runtime", name, "observed")
				}
			}
		}
	}
}

func addCommand(result *graph, file FileFact, name, category, command, status string) {
	producer := domain.ModelProducer{Kind: "algorithm", Name: "manifest-commands", Version: manifestExtractorVersion}
	id := "command:" + name
	result.addNode(domain.ModelNode{
		ID: id, Kind: "command", Abstraction: "system",
		Aspects: []string{"operations"}, Title: name, Summary: "Repository command.",
		EpistemicStatus: status,
		Evidence:        []domain.ModelEvidence{{Path: file.Path, ContentHash: file.ContentHash}},
		Producer:        producer, Attributes: map[string]string{"category": category, "command": command},
	})
	result.addEdge(edge("file:"+file.Path, "documents", id, status, producer, "file:"+file.Path))
}

func addInterface(result *graph, file FileFact, name, kind, summary string) {
	producer := domain.ModelProducer{Kind: "algorithm", Name: "interface-classifier", Version: "v1"}
	id := "interface:" + name
	result.addNode(domain.ModelNode{
		ID: id, Kind: "interface", Abstraction: "system", Aspects: []string{"api"},
		Title: name, Summary: summary, EpistemicStatus: "derived",
		Evidence: []domain.ModelEvidence{{Path: file.Path, ContentHash: file.ContentHash}},
		Producer: producer, Attributes: map[string]string{"interface_kind": kind},
	})
	result.addEdge(edge("file:"+file.Path, "exposes", id, "derived", producer, "file:"+file.Path))
}

func extractMarkdownKnowledge(result *graph, file FileFact) {
	if path.Ext(file.Path) != ".md" || len(file.Content) == 0 {
		return
	}
	producer := domain.ModelProducer{Kind: "algorithm", Name: "markdown-headings", Version: markdownExtractorVersion}
	for index, line := range strings.Split(string(file.Content), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		if title == "" {
			continue
		}
		kind := "concept"
		if strings.Contains(strings.ToLower(title), "invariant") {
			kind = "invariant"
		}
		id := kind + ":" + file.Path + "#" + slug(title)
		result.addNode(domain.ModelNode{
			ID: id, Kind: kind, Abstraction: "component", Aspects: []string{"vocabulary"},
			Title: title, Summary: "Documented repository concept.",
			EpistemicStatus: "derived",
			Evidence: []domain.ModelEvidence{{
				Path: file.Path, StartLine: uint64(index + 1), EndLine: uint64(index + 1), ContentHash: file.ContentHash,
			}},
			Producer: producer,
		})
		result.addEdge(edge("file:"+file.Path, "contains", id, "derived", producer, "file:"+file.Path))
	}
}

func slug(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(value) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			builder.WriteRune(char)
			lastDash = false
		} else if !lastDash && builder.Len() != 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func commandCategory(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "test"):
		return "test"
	case strings.Contains(lower, "lint") || strings.Contains(lower, "vet") || strings.Contains(lower, "check"):
		return "lint"
	case strings.Contains(lower, "deploy") || strings.Contains(lower, "release"):
		return "deploy"
	case strings.Contains(lower, "build") || strings.Contains(lower, "compile"):
		return "build"
	default:
		return "runtime"
	}
}

func languageForPath(filePath string) string {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".go":
		return "Go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "JavaScript"
	case ".ts":
		return "TypeScript"
	case ".tsx":
		return "TSX"
	case ".py":
		return "Python"
	default:
		return ""
	}
}

func shouldRead(filePath string) bool {
	if languageForPath(filePath) != "" {
		return true
	}
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".json", ".yaml", ".yml", ".toml", ".mod", ".sum", ".graphql", ".proto":
		return true
	default:
		return path.Base(filePath) == "Makefile"
	}
}

func isTestPath(filePath string) bool {
	base := strings.ToLower(path.Base(filePath))
	lower := strings.ToLower(filePath)
	return strings.HasSuffix(base, "_test.go") ||
		strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/")
}

func testSourceCandidates(filePath string) []string {
	dir, base := path.Dir(filePath), path.Base(filePath)
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return []string{path.Join(dir, strings.TrimSuffix(base, "_test.go")+".go")}
	case strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"):
		return []string{path.Join(dir, strings.TrimPrefix(base, "test_"))}
	case strings.HasSuffix(base, "_test.py"):
		return []string{path.Join(dir, strings.TrimSuffix(base, "_test.py")+".py")}
	}
	for _, marker := range []string{".test.", ".spec."} {
		if strings.Contains(base, marker) {
			return []string{path.Join(dir, strings.Replace(base, marker, ".", 1))}
		}
	}
	return nil
}

func safeRepositoryPath(value string) bool {
	return value != "" && value == path.Clean(value) && value != "." &&
		!path.IsAbs(value) && !strings.HasPrefix(value, "../") &&
		!strings.ContainsAny(value, "\x00\r\n\\")
}

func gitBlobHash(ctx context.Context, root, object string) (string, error) {
	command := exec.CommandContext(ctx, "git", "-C", root, "cat-file", "blob", object)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return "", err
	}
	hash, hashErr := digest.Reader(stdout)
	waitErr := command.Wait()
	if hashErr != nil {
		return "", hashErr
	}
	if waitErr != nil {
		return "", fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return hash, nil
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	data, err := gitBytes(ctx, root, args...)
	return strings.TrimSpace(string(data)), err
}

func gitBytes(ctx context.Context, root string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func loadSCIP(filePath string) ([]byte, *domain.ModelExtractor, *domain.ModelInput, error) {
	if filePath == "" {
		return nil, nil, nil, nil
	}
	absolute, err := filepath.Abs(filePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve SCIP index: %w", err)
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read SCIP index: %w", err)
	}
	hash := digest.Bytes(data)
	name, version, err := scipToolIdentity(data)
	if err != nil {
		return nil, nil, nil, err
	}
	extractor := domain.ModelExtractor{Name: "scip", Version: name + "@" + version}
	input := domain.ModelInput{Name: "scip-index", ContentHash: hash}
	return data, &extractor, &input, nil
}
