package contracts

import "github.com/bocacorazon/hermoso/internal/domain"

const (
	idPattern   = "^[a-z0-9][a-z0-9._-]{0,62}$"
	hashPattern = "^sha256:[0-9a-f]{64}$"
)

func featureDesignSchema() map[string]any {
	schema := rootSchema(FeatureDesign, []string{
		"schema_version", "context", "producer", "revision", "feature",
		"acceptance_criteria", "unresolved_questions", "complexity",
	})
	schema["properties"] = map[string]any{
		"schema_version": constant(domain.SchemaVersion),
		"context":        ref("context"),
		"producer":       ref("producer"),
		"revision":       positiveInteger(),
		"feature": object([]string{"id", "title", "objective", "target_repository"}, map[string]any{
			"id":                id(),
			"title":             nonEmptyString(),
			"objective":         nonEmptyString(),
			"target_repository": ref("target"),
		}),
		"acceptance_criteria":         nonEmptyStringArray(1),
		"constraints":                 nonEmptyStringArray(0),
		"non_goals":                   nonEmptyStringArray(0),
		"decisions":                   array(object([]string{"decision", "rationale"}, map[string]any{"decision": nonEmptyString(), "rationale": nonEmptyString()}), 0),
		"unresolved_questions":        map[string]any{"type": "array", "maxItems": 0},
		"complexity":                  map[string]any{"enum": []string{"small", "standard", "complex"}},
		"architecture_boundaries":     nonEmptyStringArray(0),
		"interfaces_and_data_changes": nonEmptyStringArray(0),
		"rollout_and_compatibility":   nonEmptyStringArray(0),
		"research_references":         array(ref("reference"), 0),
	}
	schema["$defs"] = definitions()
	return schema
}

func workGraphSchema() map[string]any {
	schema := rootSchema(WorkGraph, []string{"schema_version", "context", "producer", "revision", "items"})
	schema["properties"] = map[string]any{
		"schema_version": constant(domain.SchemaVersion),
		"context":        ref("context"),
		"producer":       ref("producer"),
		"revision":       positiveInteger(),
		"items": array(object(
			[]string{"id", "title", "prompt", "acceptance_criteria", "worker"},
			map[string]any{
				"id":                        id(),
				"title":                     nonEmptyString(),
				"prompt":                    nonEmptyString(),
				"acceptance_criteria":       nonEmptyStringArray(1),
				"parents":                   uniqueIDArray(),
				"worker":                    ref("worker"),
				"expected_changed_surfaces": nonEmptyStringArray(0),
				"validation_commands":       nonEmptyStringArray(0),
				"runtime_budget_seconds":    positiveInteger(),
			},
		), 1),
	}
	schema["$defs"] = definitions()
	return schema
}

func phaseResultSchema() map[string]any {
	schema := rootSchema(PhaseResult, []string{
		"schema_version", "context", "producer", "phase", "status", "summary", "completed_at",
	})
	schema["properties"] = map[string]any{
		"schema_version":      constant(domain.SchemaVersion),
		"context":             ref("context"),
		"producer":            ref("producer"),
		"phase":               map[string]any{"enum": []string{"design", "construction", "verification", "release"}},
		"status":              map[string]any{"enum": []string{"completed", "blocked", "failed", "cancelled"}},
		"inputs":              array(ref("reference"), 0),
		"outputs":             array(ref("reference"), 0),
		"summary":             nonEmptyString(),
		"decisions":           nonEmptyStringArray(0),
		"warnings":            nonEmptyStringArray(0),
		"unresolved_blockers": nonEmptyStringArray(0),
		"evidence":            array(ref("evidence"), 0),
		"completed_at":        map[string]any{"type": "string", "format": "date-time"},
	}
	schema["allOf"] = []any{
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"status": constant("blocked")}, "required": []string{"status"}},
			"then": map[string]any{"required": []string{"unresolved_blockers"}, "properties": map[string]any{"unresolved_blockers": map[string]any{"minItems": 1}}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"status": constant("completed")}, "required": []string{"status"}},
			"then": map[string]any{"properties": map[string]any{"unresolved_blockers": map[string]any{"maxItems": 0}}},
		},
	}
	schema["$defs"] = definitions()
	return schema
}

func rootSchema(kind Kind, required []string) map[string]any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://hermoso.dev/schemas/v1/" + string(kind) + ".json",
		"title":                string(kind),
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
	}
}

func definitions() map[string]any {
	return map[string]any{
		"context": object([]string{"schema_version", "project_id", "feature_id", "run_id", "repository"}, map[string]any{
			"schema_version": constant(domain.ContextSchemaVersion),
			"project_id":     id(), "feature_id": id(), "run_id": id(), "repository": ref("target"),
		}),
		"producer": object([]string{"skill", "runtime"}, map[string]any{
			"skill": nonEmptyString(), "runtime": nonEmptyString(), "version": nonEmptyString(),
		}),
		"target": object([]string{"repository"}, map[string]any{
			"repository": nonEmptyString(), "remote_url": nonEmptyString(), "default_branch": nonEmptyString(),
		}),
		"skill": object([]string{"name"}, map[string]any{
			"name":    map[string]any{"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$"},
			"version": nonEmptyString(),
		}),
		"worker": object([]string{"profile", "skills"}, map[string]any{
			"profile": map[string]any{"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$"},
			"skills":  array(ref("skill"), 1),
		}),
		"reference": object([]string{"context", "kind", "path", "revision", "hash"}, map[string]any{
			"context": ref("context"), "kind": nonEmptyString(), "path": nonEmptyString(), "revision": positiveInteger(),
			"hash": map[string]any{"type": "string", "pattern": hashPattern},
		}),
		"evidence": object([]string{"context", "id", "kind", "summary", "recorded_at"}, map[string]any{
			"context": ref("context"), "id": id(), "kind": nonEmptyString(), "path": nonEmptyString(), "command": nonEmptyString(),
			"summary": nonEmptyString(), "content_hash": map[string]any{"type": "string", "pattern": hashPattern},
			"recorded_at": map[string]any{"type": "string", "format": "date-time"},
		}),
	}
}

func object(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": required, "properties": properties,
	}
}

func array(items any, minimum int) map[string]any {
	result := map[string]any{"type": "array", "items": items}
	if minimum > 0 {
		result["minItems"] = minimum
	}
	return result
}

func nonEmptyStringArray(minimum int) map[string]any {
	result := array(nonEmptyString(), minimum)
	result["uniqueItems"] = true
	return result
}

func uniqueIDArray() map[string]any {
	result := array(id(), 0)
	result["uniqueItems"] = true
	return result
}

func nonEmptyString() map[string]any {
	return map[string]any{"type": "string", "minLength": 1}
}

func id() map[string]any {
	return map[string]any{"type": "string", "pattern": idPattern}
}

func positiveInteger() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1}
}

func constant(value string) map[string]any {
	return map[string]any{"const": value}
}

func ref(name string) map[string]any {
	return map[string]any{"$ref": "#/$defs/" + name}
}
