package model

import (
	"context"
	"fmt"
	"path"
	"strings"
	"unicode"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tsjavascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tstypescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/bocacorazon/hermoso/internal/domain"
)

type languageConfig struct {
	language         *sitter.Language
	declarationKinds map[string]bool
	importKinds      map[string]bool
}

func extractStructure(ctx context.Context, result *graph, file FileFact, files []FileFact) error {
	config, err := parserConfig(file.Language)
	if err != nil {
		return err
	}
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(config.language); err != nil {
		return err
	}
	tree := parser.ParseCtx(ctx, file.Content, nil)
	if tree == nil {
		return errorsForContext(ctx, "Tree-sitter returned no syntax tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		node := result.nodes["file:"+file.Path]
		node.Attributes["parse_errors"] = "true"
		result.nodes[node.ID] = node
	}
	paths := make(map[string]struct{}, len(files))
	for _, candidate := range files {
		paths[candidate.Path] = struct{}{}
	}
	walkSyntax(result, file, root, config, paths)
	return nil
}

func walkSyntax(result *graph, file FileFact, node *sitter.Node, config languageConfig, files map[string]struct{}) {
	if config.declarationKinds[node.Kind()] {
		addDeclaration(result, file, node)
	}
	if config.importKinds[node.Kind()] {
		for _, specifier := range importSpecifiers(file.Content, node) {
			addImport(result, file, specifier, files)
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		child := node.NamedChild(index)
		if child != nil {
			walkSyntax(result, file, child, config, files)
		}
	}
}

func addDeclaration(result *graph, file FileFact, node *sitter.Node) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := sourceText(file.Content, nameNode)
	if name == "" || len(name) > 256 {
		return
	}
	start, end := node.StartPosition(), node.EndPosition()
	producer := domain.ModelProducer{Kind: "extractor", Name: "tree-sitter-" + strings.ToLower(file.Language), Version: structureExtractorVersion}
	id := fmt.Sprintf("symbol:%s#%s:%s@%d", file.Path, node.Kind(), name, start.Row+1)
	result.addNode(domain.ModelNode{
		ID: id, Kind: "symbol", Abstraction: "code", Aspects: []string{"structure"},
		Title: name, Summary: "Source declaration.", EpistemicStatus: "observed",
		Evidence: []domain.ModelEvidence{{
			Path: file.Path, StartLine: uint64(start.Row + 1), EndLine: uint64(end.Row + 1), ContentHash: file.ContentHash,
		}},
		Producer: producer, Attributes: map[string]string{"declaration_kind": node.Kind(), "language": file.Language},
	})
	result.addEdge(edge("file:"+file.Path, "contains", id, "observed", producer, "file:"+file.Path))

	if (file.Language == "TSX" || file.Language == "JavaScript") && strings.HasSuffix(strings.ToLower(file.Path), "x") {
		first, _ := utf8First(name)
		if unicode.IsUpper(first) {
			addSymbolInterface(result, file, id, name, "ui")
		}
	}
	if name == "ServeHTTP" || strings.Contains(strings.ToLower(name), "handler") || strings.Contains(strings.ToLower(name), "route") {
		addSymbolInterface(result, file, id, name, "api")
	}
}

func addSymbolInterface(result *graph, file FileFact, symbolID, name, kind string) {
	producer := domain.ModelProducer{Kind: "algorithm", Name: "symbol-interface-classifier", Version: "v1"}
	id := "interface:" + stableID(kind, file.Path, symbolID)
	result.addNode(domain.ModelNode{
		ID: id, Kind: "interface", Abstraction: "component", Aspects: []string{"api"},
		Title: name, Summary: "Interaction surface derived from a source declaration.",
		EpistemicStatus: "derived", DerivedFrom: []string{symbolID}, Producer: producer,
		Attributes: map[string]string{"interface_kind": kind},
	})
	result.addEdge(edge(symbolID, "exposes", id, "derived", producer, symbolID))
}

func addImport(result *graph, file FileFact, specifier string, files map[string]struct{}) {
	specifier = strings.Trim(strings.TrimSpace(specifier), `"'`)
	if specifier == "" {
		return
	}
	producer := domain.ModelProducer{Kind: "extractor", Name: "tree-sitter-" + strings.ToLower(file.Language), Version: structureExtractorVersion}
	target := resolveImport(file.Path, specifier, file.Language, files)
	if target == "" {
		target = "component:dependency:" + stableID("dependency", file.Language, specifier)
		result.addNode(domain.ModelNode{
			ID: target, Kind: "component", Abstraction: "component", Aspects: []string{"structure"},
			Title: specifier, Summary: "Imported dependency.", EpistemicStatus: "observed",
			Evidence: []domain.ModelEvidence{{Path: file.Path, ContentHash: file.ContentHash}},
			Producer: producer, Attributes: map[string]string{"specifier": specifier, "language": file.Language},
		})
	}
	result.addEdge(edge("file:"+file.Path, "imports", target, "observed", producer, "file:"+file.Path))
}

func importSpecifiers(content []byte, node *sitter.Node) []string {
	for _, field := range []string{"path", "source", "module_name"} {
		if child := node.ChildByFieldName(field); child != nil {
			return []string{sourceText(content, child)}
		}
	}
	var result []string
	collectImportNames(content, node, &result)
	return result
}

func collectImportNames(content []byte, node *sitter.Node, result *[]string) {
	if node.Kind() == "dotted_name" || node.Kind() == "string" {
		*result = append(*result, sourceText(content, node))
		return
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if child := node.NamedChild(index); child != nil {
			collectImportNames(content, child, result)
		}
	}
}

func resolveImport(sourcePath, specifier, language string, files map[string]struct{}) string {
	var candidates []string
	dir := path.Dir(sourcePath)
	switch language {
	case "JavaScript", "TypeScript", "TSX":
		if !strings.HasPrefix(specifier, ".") {
			return ""
		}
		base := path.Clean(path.Join(dir, specifier))
		candidates = append(candidates, base)
		for _, extension := range []string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs"} {
			candidates = append(candidates, base+extension, path.Join(base, "index"+extension))
		}
	case "Python":
		base := strings.ReplaceAll(specifier, ".", "/")
		candidates = []string{base + ".py", path.Join(base, "__init__.py"), path.Join(dir, base+".py")}
	default:
		return ""
	}
	for _, candidate := range candidates {
		if _, ok := files[candidate]; ok {
			return "file:" + candidate
		}
	}
	return ""
}

func classifyFileInterface(result *graph, file FileFact) {
	lower := strings.ToLower(file.Path)
	switch {
	case file.Language == "Go" && (path.Base(file.Path) == "main.go" || strings.HasPrefix(lower, "cmd/")) && bytesContains(file.Content, "package main"):
		addInterface(result, file, strings.TrimSuffix(strings.ReplaceAll(file.Path, "/", "-"), path.Ext(file.Path)), "cli", "Go command entry point.")
	case strings.HasSuffix(lower, ".graphql") || strings.HasSuffix(lower, ".proto") ||
		strings.Contains(lower, "openapi") || strings.Contains(lower, "swagger"):
		addInterface(result, file, strings.ReplaceAll(file.Path, "/", "-"), "api", "Declared protocol or API schema.")
	}
}

func parserConfig(language string) (languageConfig, error) {
	switch language {
	case "Go":
		return languageConfig{
			language:         sitter.NewLanguage(tsgo.Language()),
			declarationKinds: set("function_declaration", "method_declaration", "type_spec", "const_spec", "var_spec"),
			importKinds:      set("import_spec"),
		}, nil
	case "JavaScript":
		return languageConfig{
			language:         sitter.NewLanguage(tsjavascript.Language()),
			declarationKinds: set("function_declaration", "class_declaration", "method_definition", "variable_declarator"),
			importKinds:      set("import_statement", "export_statement"),
		}, nil
	case "TypeScript":
		return languageConfig{
			language:         sitter.NewLanguage(tstypescript.LanguageTypescript()),
			declarationKinds: set("function_declaration", "class_declaration", "method_definition", "variable_declarator", "interface_declaration", "type_alias_declaration", "enum_declaration"),
			importKinds:      set("import_statement", "export_statement"),
		}, nil
	case "TSX":
		return languageConfig{
			language:         sitter.NewLanguage(tstypescript.LanguageTSX()),
			declarationKinds: set("function_declaration", "class_declaration", "method_definition", "variable_declarator", "interface_declaration", "type_alias_declaration", "enum_declaration"),
			importKinds:      set("import_statement", "export_statement"),
		}, nil
	case "Python":
		return languageConfig{
			language:         sitter.NewLanguage(tspython.Language()),
			declarationKinds: set("function_definition", "class_definition"),
			importKinds:      set("import_statement", "import_from_statement"),
		}, nil
	default:
		return languageConfig{}, fmt.Errorf("unsupported language %q", language)
	}
}

func sourceText(content []byte, node *sitter.Node) string {
	start, end := node.StartByte(), node.EndByte()
	if start >= uint(len(content)) || end > uint(len(content)) || end <= start {
		return ""
	}
	return strings.TrimSpace(string(content[start:end]))
}

func set(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func errorsForContext(ctx context.Context, fallback string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("%s", fallback)
}

func bytesContains(content []byte, value string) bool {
	return strings.Contains(string(content), value)
}

func utf8First(value string) (rune, int) {
	for _, char := range value {
		return char, 1
	}
	return 0, 0
}
