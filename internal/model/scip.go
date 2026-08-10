package model

import (
	"errors"
	"fmt"
	"path"
	"strings"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func scipToolIdentity(data []byte) (string, string, error) {
	var index scip.Index
	if err := proto.Unmarshal(data, &index); err != nil {
		return "", "", fmt.Errorf("decode SCIP index: %w", err)
	}
	if index.Metadata == nil || index.Metadata.ToolInfo == nil ||
		strings.TrimSpace(index.Metadata.ToolInfo.Name) == "" ||
		strings.TrimSpace(index.Metadata.ToolInfo.Version) == "" {
		return "", "", errors.New("SCIP index must declare tool name and version")
	}
	return index.Metadata.ToolInfo.Name, index.Metadata.ToolInfo.Version, nil
}

func augmentSCIP(result *graph, data []byte, files []FileFact) error {
	var index scip.Index
	if err := proto.Unmarshal(data, &index); err != nil {
		return fmt.Errorf("decode SCIP index: %w", err)
	}
	fileFacts := make(map[string]FileFact, len(files))
	for _, file := range files {
		fileFacts[file.Path] = file
	}
	toolName, toolVersion := "scip", "unknown"
	if index.Metadata != nil && index.Metadata.ToolInfo != nil {
		if index.Metadata.ToolInfo.Name != "" {
			toolName = index.Metadata.ToolInfo.Name
		}
		if index.Metadata.ToolInfo.Version != "" {
			toolVersion = index.Metadata.ToolInfo.Version
		}
	}
	producer := domain.ModelProducer{Kind: "extractor", Name: "scip-" + toolName, Version: toolVersion}
	names := map[string]string{}
	for _, document := range index.Documents {
		for _, information := range document.Symbols {
			if information.Symbol != "" {
				names[information.Symbol] = information.DisplayName
			}
		}
	}
	for _, information := range index.ExternalSymbols {
		if information.Symbol != "" {
			names[information.Symbol] = information.DisplayName
		}
	}
	for _, document := range index.Documents {
		filePath := path.Clean(document.RelativePath)
		file, ok := fileFacts[filePath]
		if !ok || !safeRepositoryPath(filePath) {
			return fmt.Errorf("SCIP document %q does not resolve to the indexed Git tree", document.RelativePath)
		}
		for _, occurrence := range document.Occurrences {
			if occurrence.Symbol == "" {
				continue
			}
			sourceRange, ok := occurrence.SourceRange()
			if !ok || sourceRange.Start.Line < 0 || sourceRange.End.Line < sourceRange.Start.Line {
				return fmt.Errorf("SCIP occurrence in %s has an invalid source range", filePath)
			}
			symbolID := "symbol:scip:" + occurrence.Symbol
			title := names[occurrence.Symbol]
			if title == "" {
				title = scipSymbolTitle(occurrence.Symbol)
			}
			status := "derived"
			summary := "SCIP symbol reference."
			if scip.SymbolRole_Definition.Matches(occurrence) {
				status = "observed"
				summary = "Compiler-indexed symbol definition."
			}
			result.addNode(domain.ModelNode{
				ID: symbolID, Kind: "symbol", Abstraction: "code", Aspects: []string{"structure"},
				Title: title, Summary: summary, EpistemicStatus: status,
				Evidence: []domain.ModelEvidence{{
					Path: filePath, StartLine: uint64(sourceRange.Start.Line + 1),
					EndLine: uint64(sourceRange.End.Line + 1), ContentHash: file.ContentHash,
				}},
				Producer: producer, Attributes: map[string]string{"scip_symbol": occurrence.Symbol, "language": document.Language},
			})
			relation := "references"
			if scip.SymbolRole_Definition.Matches(occurrence) {
				relation = "contains"
			}
			result.addEdge(edge("file:"+filePath, relation, symbolID, status, producer, "file:"+filePath))
		}
	}
	return nil
}

func scipSymbolTitle(symbol string) string {
	symbol = strings.TrimSpace(symbol)
	if index := strings.LastIndexAny(symbol, "/#."); index >= 0 && index+1 < len(symbol) {
		return symbol[index+1:]
	}
	return symbol
}
