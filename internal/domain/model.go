package domain

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bocacorazon/hermoso/internal/digest"
)

const ModelSchemaVersion = "hermoso-repository-model/v1"

var gitObjectPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type ModelProducer struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ModelExtractor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ModelInput struct {
	Name        string `json:"name"`
	ContentHash string `json:"content_hash"`
}

type ModelManifest struct {
	SchemaVersion     string           `json:"schema_version"`
	SnapshotID        string           `json:"snapshot_id"`
	ContentHash       string           `json:"content_hash"`
	ProjectID         string           `json:"project_id"`
	Repository        TargetIdentity   `json:"repository"`
	SourceRevision    string           `json:"source_revision"`
	VocabularyVersion string           `json:"vocabulary_version"`
	GeneratedAt       time.Time        `json:"generated_at"`
	Extractors        []ModelExtractor `json:"extractors"`
	Inputs            []ModelInput     `json:"inputs,omitempty"`
}

type ModelVocabulary struct {
	SchemaVersion     string              `json:"schema_version"`
	Version           string              `json:"version"`
	NodeKinds         []string            `json:"node_kinds"`
	Abstractions      []string            `json:"abstractions"`
	Aspects           []string            `json:"aspects"`
	EpistemicStatuses []string            `json:"epistemic_statuses"`
	Relations         []string            `json:"relations"`
	InterfaceKinds    []string            `json:"interface_kinds"`
	Extensions        map[string][]string `json:"extensions,omitempty"`
}

type ModelEvidence struct {
	Path        string `json:"path"`
	StartLine   uint64 `json:"start_line,omitempty"`
	EndLine     uint64 `json:"end_line,omitempty"`
	ContentHash string `json:"content_hash"`
}

type ModelNode struct {
	ID              string            `json:"id"`
	Kind            string            `json:"kind"`
	Abstraction     string            `json:"abstraction"`
	Aspects         []string          `json:"aspects"`
	Title           string            `json:"title"`
	Summary         string            `json:"summary"`
	EpistemicStatus string            `json:"epistemic_status"`
	Evidence        []ModelEvidence   `json:"evidence,omitempty"`
	Producer        ModelProducer     `json:"producer"`
	DerivedFrom     []string          `json:"derived_from,omitempty"`
	Attributes      map[string]string `json:"attributes,omitempty"`
}

type ModelEdge struct {
	ID              string        `json:"id"`
	Source          string        `json:"source"`
	Relation        string        `json:"relation"`
	Target          string        `json:"target"`
	EpistemicStatus string        `json:"epistemic_status"`
	Evidence        []string      `json:"evidence,omitempty"`
	Producer        ModelProducer `json:"producer"`
}

type ModelSnapshot struct {
	Manifest   ModelManifest     `json:"manifest"`
	Vocabulary ModelVocabulary   `json:"vocabulary"`
	Nodes      []ModelNode       `json:"nodes"`
	Edges      []ModelEdge       `json:"edges"`
	Views      map[string]string `json:"views"`
}

type ModelPointer struct {
	SchemaVersion  string         `json:"schema_version"`
	ProjectID      string         `json:"project_id"`
	Repository     TargetIdentity `json:"repository"`
	SnapshotID     string         `json:"snapshot_id"`
	ContentHash    string         `json:"content_hash"`
	SourceRevision string         `json:"source_revision"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type modelHashPayload struct {
	SchemaVersion     string            `json:"schema_version"`
	ProjectID         string            `json:"project_id"`
	Repository        TargetIdentity    `json:"repository"`
	SourceRevision    string            `json:"source_revision"`
	VocabularyVersion string            `json:"vocabulary_version"`
	Extractors        []ModelExtractor  `json:"extractors"`
	Inputs            []ModelInput      `json:"inputs,omitempty"`
	Vocabulary        ModelVocabulary   `json:"vocabulary"`
	Nodes             []ModelNode       `json:"nodes"`
	Edges             []ModelEdge       `json:"edges"`
	Views             map[string]string `json:"views"`
}

func DefaultModelVocabulary() ModelVocabulary {
	return ModelVocabulary{
		SchemaVersion: ModelSchemaVersion,
		Version:       "v1",
		NodeKinds: []string{
			"repository", "file", "symbol", "component", "domain", "interface",
			"test", "command", "concept", "invariant", "decision", "requirement", "scenario",
		},
		Abstractions:      []string{"system", "container", "component", "code"},
		Aspects:           []string{"structure", "runtime", "data", "api", "testing", "operations", "security", "vocabulary"},
		EpistemicStatuses: []string{"observed", "derived", "draft", "working", "stable", "canonical", "deprecated", "archived"},
		Relations: []string{
			"contains", "imports", "references", "depends_on", "exposes", "exercises",
			"verifies", "governed_by", "uses_concept", "derived_from", "implements", "documents",
		},
		InterfaceKinds: []string{"api", "cli", "ui", "file", "event", "library"},
	}
}

func FinalizeModelSnapshot(snapshot ModelSnapshot) (ModelSnapshot, error) {
	normalizeModelSnapshot(&snapshot)
	hash, err := ModelSnapshotHash(snapshot)
	if err != nil {
		return ModelSnapshot{}, fmt.Errorf("hash model snapshot: %w", err)
	}
	snapshot.Manifest.ContentHash = hash
	snapshot.Manifest.SnapshotID = "model-" + strings.TrimPrefix(hash, "sha256:")[:32]
	if err := snapshot.Validate(); err != nil {
		return ModelSnapshot{}, err
	}
	return snapshot, nil
}

func ModelSnapshotHash(snapshot ModelSnapshot) (string, error) {
	copy := cloneModelSnapshot(snapshot)
	normalizeModelSnapshot(&copy)
	payload := modelHashPayload{
		SchemaVersion:     copy.Manifest.SchemaVersion,
		ProjectID:         copy.Manifest.ProjectID,
		Repository:        copy.Manifest.Repository,
		SourceRevision:    copy.Manifest.SourceRevision,
		VocabularyVersion: copy.Manifest.VocabularyVersion,
		Extractors:        copy.Manifest.Extractors,
		Inputs:            copy.Manifest.Inputs,
		Vocabulary:        copy.Vocabulary,
		Nodes:             copy.Nodes,
		Edges:             copy.Edges,
		Views:             copy.Views,
	}
	return digest.JSON(payload)
}

func (s ModelSnapshot) Validate() error {
	var errs ValidationErrors
	s.Manifest.validate("manifest", &errs)
	s.Vocabulary.validate("vocabulary", &errs)
	if s.Manifest.VocabularyVersion != s.Vocabulary.Version {
		errs.add("manifest.vocabulary_version", "must match vocabulary.version")
	}
	if len(s.Nodes) == 0 {
		errs.add("nodes", "must contain at least one node")
	}
	if s.Views == nil {
		errs.add("views", "must be set")
	}
	for _, required := range requiredModelViews() {
		if strings.TrimSpace(s.Views[required]) == "" {
			errs.add("views."+required, "must contain a rendered view")
		}
	}

	nodeIDs := make(map[string]struct{}, len(s.Nodes))
	for i, node := range s.Nodes {
		itemPath := fmt.Sprintf("nodes[%d]", i)
		node.validate(itemPath, s.Vocabulary, &errs)
		if _, exists := nodeIDs[node.ID]; exists {
			errs.add(itemPath+".id", "must be unique")
		}
		nodeIDs[node.ID] = struct{}{}
	}
	for i, node := range s.Nodes {
		for j, source := range node.DerivedFrom {
			if _, ok := nodeIDs[source]; !ok {
				errs.add(fmt.Sprintf("nodes[%d].derived_from[%d]", i, j), "references an unknown node")
			}
		}
	}

	edgeIDs := make(map[string]struct{}, len(s.Edges))
	for i, edge := range s.Edges {
		itemPath := fmt.Sprintf("edges[%d]", i)
		edge.validate(itemPath, s.Vocabulary, &errs)
		if _, exists := edgeIDs[edge.ID]; exists {
			errs.add(itemPath+".id", "must be unique")
		}
		edgeIDs[edge.ID] = struct{}{}
		if _, ok := nodeIDs[edge.Source]; !ok {
			errs.add(itemPath+".source", "references an unknown node")
		}
		if _, ok := nodeIDs[edge.Target]; !ok {
			errs.add(itemPath+".target", "references an unknown node")
		}
		for j, evidence := range edge.Evidence {
			if _, ok := nodeIDs[evidence]; !ok {
				errs.add(fmt.Sprintf("%s.evidence[%d]", itemPath, j), "references an unknown node")
			}
		}
	}
	for name, content := range s.Views {
		if !validModelRelativePath(name) {
			errs.add("views."+name, "must be a clean relative path")
		}
		if strings.TrimSpace(content) == "" {
			errs.add("views."+name, "must not be blank")
		}
	}

	if hash, err := ModelSnapshotHash(s); err != nil {
		errs.add("manifest.content_hash", err.Error())
	} else if s.Manifest.ContentHash != hash {
		errs.add("manifest.content_hash", "does not match snapshot content")
	} else if s.Manifest.SnapshotID != "model-"+strings.TrimPrefix(hash, "sha256:")[:32] {
		errs.add("manifest.snapshot_id", "does not match snapshot content hash")
	}
	return validationResult(errs)
}

func (m ModelManifest) validate(itemPath string, errs *ValidationErrors) {
	if m.SchemaVersion != ModelSchemaVersion {
		errs.add(itemPath+".schema_version", "must be "+ModelSchemaVersion)
	}
	if !strings.HasPrefix(m.SnapshotID, "model-") || len(m.SnapshotID) != 38 {
		errs.add(itemPath+".snapshot_id", "must be model- followed by 32 lowercase hexadecimal characters")
	}
	if !hashPattern.MatchString(m.ContentHash) {
		errs.add(itemPath+".content_hash", "must use sha256:<64 lowercase hex characters>")
	}
	validateID(itemPath+".project_id", m.ProjectID, errs)
	m.Repository.validate(itemPath+".repository", errs)
	if !gitObjectPattern.MatchString(m.SourceRevision) {
		errs.add(itemPath+".source_revision", "must be a full lowercase Git object ID")
	}
	validateRequired(itemPath+".vocabulary_version", m.VocabularyVersion, errs)
	if m.GeneratedAt.IsZero() {
		errs.add(itemPath+".generated_at", "must be set")
	}
	if len(m.Extractors) == 0 {
		errs.add(itemPath+".extractors", "must contain at least one extractor")
	}
	seen := map[string]struct{}{}
	for i, extractor := range m.Extractors {
		extractorPath := fmt.Sprintf("%s.extractors[%d]", itemPath, i)
		validateRequired(extractorPath+".name", extractor.Name, errs)
		validateRequired(extractorPath+".version", extractor.Version, errs)
		if _, ok := seen[extractor.Name]; ok {
			errs.add(extractorPath+".name", "must be unique")
		}
		seen[extractor.Name] = struct{}{}
	}
	seen = map[string]struct{}{}
	for i, input := range m.Inputs {
		inputPath := fmt.Sprintf("%s.inputs[%d]", itemPath, i)
		validateRequired(inputPath+".name", input.Name, errs)
		if !hashPattern.MatchString(input.ContentHash) {
			errs.add(inputPath+".content_hash", "must use sha256:<64 lowercase hex characters>")
		}
		if _, ok := seen[input.Name]; ok {
			errs.add(inputPath+".name", "must be unique")
		}
		seen[input.Name] = struct{}{}
	}
}

func (v ModelVocabulary) validate(itemPath string, errs *ValidationErrors) {
	if v.SchemaVersion != ModelSchemaVersion {
		errs.add(itemPath+".schema_version", "must be "+ModelSchemaVersion)
	}
	validateRequired(itemPath+".version", v.Version, errs)
	validateVocabularyList(itemPath+".node_kinds", v.NodeKinds, &[]string{"repository", "file", "symbol", "interface", "test", "command", "concept", "invariant", "requirement", "scenario"}, errs)
	validateVocabularyList(itemPath+".abstractions", v.Abstractions, &[]string{"system", "container", "component", "code"}, errs)
	validateVocabularyList(itemPath+".aspects", v.Aspects, &[]string{"structure", "runtime", "data", "api", "testing", "operations", "security", "vocabulary"}, errs)
	validateVocabularyList(itemPath+".epistemic_statuses", v.EpistemicStatuses, &[]string{"observed", "derived", "draft", "working", "stable", "canonical", "deprecated", "archived"}, errs)
	validateVocabularyList(itemPath+".relations", v.Relations, &[]string{"contains", "imports", "references", "depends_on", "exposes", "exercises", "verifies", "governed_by", "uses_concept", "derived_from"}, errs)
	validateVocabularyList(itemPath+".interface_kinds", v.InterfaceKinds, &[]string{"api", "cli", "ui", "file", "event", "library"}, errs)
	for name, values := range v.Extensions {
		if strings.TrimSpace(name) == "" {
			errs.add(itemPath+".extensions", "keys must not be blank")
		}
		validateVocabularyList(itemPath+".extensions."+name, values, nil, errs)
	}
}

func (n ModelNode) validate(itemPath string, vocabulary ModelVocabulary, errs *ValidationErrors) {
	validateModelIdentifier(itemPath+".id", n.ID, errs)
	validateMember(itemPath+".kind", n.Kind, vocabulary.NodeKinds, errs)
	validateMember(itemPath+".abstraction", n.Abstraction, vocabulary.Abstractions, errs)
	validateMembers(itemPath+".aspects", n.Aspects, vocabulary.Aspects, true, errs)
	validateRequired(itemPath+".title", n.Title, errs)
	validateRequired(itemPath+".summary", n.Summary, errs)
	validateMember(itemPath+".epistemic_status", n.EpistemicStatus, vocabulary.EpistemicStatuses, errs)
	n.Producer.validate(itemPath+".producer", errs)
	if len(n.Evidence) == 0 && len(n.DerivedFrom) == 0 {
		errs.add(itemPath, "must include evidence or derived_from")
	}
	for i, evidence := range n.Evidence {
		evidence.validate(fmt.Sprintf("%s.evidence[%d]", itemPath, i), errs)
	}
	validateModelIDList(itemPath+".derived_from", n.DerivedFrom, errs)
	for key, value := range n.Attributes {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			errs.add(itemPath+".attributes", "keys and values must not be blank")
		}
	}
	if n.Kind == "interface" {
		validateMember(itemPath+".attributes.interface_kind", n.Attributes["interface_kind"], vocabulary.InterfaceKinds, errs)
	}
}

func (e ModelEdge) validate(itemPath string, vocabulary ModelVocabulary, errs *ValidationErrors) {
	validateModelIdentifier(itemPath+".id", e.ID, errs)
	validateModelIdentifier(itemPath+".source", e.Source, errs)
	validateMember(itemPath+".relation", e.Relation, vocabulary.Relations, errs)
	validateModelIdentifier(itemPath+".target", e.Target, errs)
	if e.Source == e.Target {
		errs.add(itemPath, "source and target must differ")
	}
	validateMember(itemPath+".epistemic_status", e.EpistemicStatus, vocabulary.EpistemicStatuses, errs)
	validateModelIDList(itemPath+".evidence", e.Evidence, errs)
	e.Producer.validate(itemPath+".producer", errs)
}

func (p ModelProducer) validate(itemPath string, errs *ValidationErrors) {
	validateMember(itemPath+".kind", p.Kind, []string{"extractor", "algorithm", "human", "agent"}, errs)
	validateRequired(itemPath+".name", p.Name, errs)
	validateRequired(itemPath+".version", p.Version, errs)
}

func (e ModelEvidence) validate(itemPath string, errs *ValidationErrors) {
	if !validModelRelativePath(e.Path) {
		errs.add(itemPath+".path", "must be a clean repository-relative path")
	}
	if e.StartLine == 0 && e.EndLine != 0 {
		errs.add(itemPath+".start_line", "must be set when end_line is set")
	}
	if e.EndLine != 0 && e.EndLine < e.StartLine {
		errs.add(itemPath+".end_line", "must not precede start_line")
	}
	if !hashPattern.MatchString(e.ContentHash) {
		errs.add(itemPath+".content_hash", "must use sha256:<64 lowercase hex characters>")
	}
}

func (p ModelPointer) Validate() error {
	var errs ValidationErrors
	if p.SchemaVersion != ModelSchemaVersion {
		errs.add("schema_version", "must be "+ModelSchemaVersion)
	}
	validateID("project_id", p.ProjectID, &errs)
	p.Repository.validate("repository", &errs)
	if !strings.HasPrefix(p.SnapshotID, "model-") || len(p.SnapshotID) != 38 {
		errs.add("snapshot_id", "must be model- followed by 32 lowercase hexadecimal characters")
	}
	if !hashPattern.MatchString(p.ContentHash) {
		errs.add("content_hash", "must use sha256:<64 lowercase hex characters>")
	}
	if !gitObjectPattern.MatchString(p.SourceRevision) {
		errs.add("source_revision", "must be a full lowercase Git object ID")
	}
	if p.UpdatedAt.IsZero() {
		errs.add("updated_at", "must be set")
	}
	return validationResult(errs)
}

func requiredModelViews() []string {
	return []string{
		"index.md",
		"views/system-context.md",
		"views/domains.md",
		"views/components.md",
		"views/runtime.md",
		"views/data.md",
		"views/interfaces.md",
		"views/testing.md",
		"views/operations.md",
		"views/decisions.md",
		"views/vocabulary.md",
	}
}

func normalizeModelSnapshot(snapshot *ModelSnapshot) {
	sort.Slice(snapshot.Manifest.Extractors, func(i, j int) bool {
		return snapshot.Manifest.Extractors[i].Name < snapshot.Manifest.Extractors[j].Name
	})
	sort.Slice(snapshot.Manifest.Inputs, func(i, j int) bool {
		return snapshot.Manifest.Inputs[i].Name < snapshot.Manifest.Inputs[j].Name
	})
	normalizeVocabulary(&snapshot.Vocabulary)
	for i := range snapshot.Nodes {
		sort.Strings(snapshot.Nodes[i].Aspects)
		sort.Strings(snapshot.Nodes[i].DerivedFrom)
		sort.Slice(snapshot.Nodes[i].Evidence, func(a, b int) bool {
			left, right := snapshot.Nodes[i].Evidence[a], snapshot.Nodes[i].Evidence[b]
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			return left.StartLine < right.StartLine
		})
	}
	sort.Slice(snapshot.Nodes, func(i, j int) bool { return snapshot.Nodes[i].ID < snapshot.Nodes[j].ID })
	for i := range snapshot.Edges {
		sort.Strings(snapshot.Edges[i].Evidence)
	}
	sort.Slice(snapshot.Edges, func(i, j int) bool { return snapshot.Edges[i].ID < snapshot.Edges[j].ID })
}

func cloneModelSnapshot(snapshot ModelSnapshot) ModelSnapshot {
	copy := snapshot
	copy.Manifest.Extractors = append([]ModelExtractor(nil), snapshot.Manifest.Extractors...)
	copy.Manifest.Inputs = append([]ModelInput(nil), snapshot.Manifest.Inputs...)
	copy.Vocabulary.NodeKinds = append([]string(nil), snapshot.Vocabulary.NodeKinds...)
	copy.Vocabulary.Abstractions = append([]string(nil), snapshot.Vocabulary.Abstractions...)
	copy.Vocabulary.Aspects = append([]string(nil), snapshot.Vocabulary.Aspects...)
	copy.Vocabulary.EpistemicStatuses = append([]string(nil), snapshot.Vocabulary.EpistemicStatuses...)
	copy.Vocabulary.Relations = append([]string(nil), snapshot.Vocabulary.Relations...)
	copy.Vocabulary.InterfaceKinds = append([]string(nil), snapshot.Vocabulary.InterfaceKinds...)
	if snapshot.Vocabulary.Extensions != nil {
		copy.Vocabulary.Extensions = make(map[string][]string, len(snapshot.Vocabulary.Extensions))
		for key, values := range snapshot.Vocabulary.Extensions {
			copy.Vocabulary.Extensions[key] = append([]string(nil), values...)
		}
	}
	copy.Nodes = make([]ModelNode, len(snapshot.Nodes))
	for i, node := range snapshot.Nodes {
		copy.Nodes[i] = node
		copy.Nodes[i].Aspects = append([]string(nil), node.Aspects...)
		copy.Nodes[i].Evidence = append([]ModelEvidence(nil), node.Evidence...)
		copy.Nodes[i].DerivedFrom = append([]string(nil), node.DerivedFrom...)
		if node.Attributes != nil {
			copy.Nodes[i].Attributes = make(map[string]string, len(node.Attributes))
			for key, value := range node.Attributes {
				copy.Nodes[i].Attributes[key] = value
			}
		}
	}
	copy.Edges = make([]ModelEdge, len(snapshot.Edges))
	for i, edge := range snapshot.Edges {
		copy.Edges[i] = edge
		copy.Edges[i].Evidence = append([]string(nil), edge.Evidence...)
	}
	if snapshot.Views != nil {
		copy.Views = make(map[string]string, len(snapshot.Views))
		for key, value := range snapshot.Views {
			copy.Views[key] = value
		}
	}
	return copy
}

func normalizeVocabulary(vocabulary *ModelVocabulary) {
	sort.Strings(vocabulary.NodeKinds)
	sort.Strings(vocabulary.Abstractions)
	sort.Strings(vocabulary.Aspects)
	sort.Strings(vocabulary.EpistemicStatuses)
	sort.Strings(vocabulary.Relations)
	sort.Strings(vocabulary.InterfaceKinds)
	for key := range vocabulary.Extensions {
		sort.Strings(vocabulary.Extensions[key])
	}
}

func validateVocabularyList(itemPath string, values []string, required *[]string, errs *ValidationErrors) {
	if len(values) == 0 {
		errs.add(itemPath, "must contain at least one value")
		return
	}
	seen := map[string]struct{}{}
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			errs.add(fmt.Sprintf("%s[%d]", itemPath, i), "must not be blank")
		}
		if _, ok := seen[value]; ok {
			errs.add(fmt.Sprintf("%s[%d]", itemPath, i), "must be unique")
		}
		seen[value] = struct{}{}
	}
	if required != nil {
		for _, value := range *required {
			if _, ok := seen[value]; !ok {
				errs.add(itemPath, "must contain core value "+value)
			}
		}
	}
}

func validateModelIdentifier(itemPath, value string, errs *ValidationErrors) {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 1024 || strings.ContainsAny(value, "\r\n\x00") {
		errs.add(itemPath, "must be a non-empty stable identifier without surrounding whitespace or control characters")
	}
}

func validateModelIDList(itemPath string, values []string, errs *ValidationErrors) {
	seen := map[string]struct{}{}
	for i, value := range values {
		validateModelIdentifier(fmt.Sprintf("%s[%d]", itemPath, i), value, errs)
		if _, ok := seen[value]; ok {
			errs.add(fmt.Sprintf("%s[%d]", itemPath, i), "must be unique")
		}
		seen[value] = struct{}{}
	}
}

func validateMembers(itemPath string, values, allowed []string, required bool, errs *ValidationErrors) {
	if required && len(values) == 0 {
		errs.add(itemPath, "must contain at least one value")
	}
	seen := map[string]struct{}{}
	for i, value := range values {
		validateMember(fmt.Sprintf("%s[%d]", itemPath, i), value, allowed, errs)
		if _, ok := seen[value]; ok {
			errs.add(fmt.Sprintf("%s[%d]", itemPath, i), "must be unique")
		}
		seen[value] = struct{}{}
	}
}

func validateMember(itemPath, value string, allowed []string, errs *ValidationErrors) {
	for _, candidate := range allowed {
		if value == candidate {
			return
		}
	}
	errs.add(itemPath, "is not in the versioned model vocabulary")
}

func validModelRelativePath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || path.IsAbs(value) {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}
