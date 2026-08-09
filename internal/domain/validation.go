package domain

import (
	"fmt"
	"regexp"
	"strings"
)

const SchemaVersion = "1"
const ContextSchemaVersion = "hermoso-context/v1"

var (
	idPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
	hashPattern      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	gitCommitPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	profilePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
)

type ValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	parts := make([]string, len(e))
	for i, item := range e {
		parts[i] = item.Path + ": " + item.Message
	}
	return strings.Join(parts, "; ")
}

func (e ValidationErrors) ValidationDetails() []map[string]string {
	details := make([]map[string]string, len(e))
	for i, item := range e {
		details[i] = map[string]string{"path": item.Path, "message": item.Message}
	}
	return details
}

func (e *ValidationErrors) add(path, message string) {
	*e = append(*e, ValidationError{Path: path, Message: message})
}

func validateVersion(version string, errs *ValidationErrors) {
	if version != SchemaVersion {
		errs.add("schema_version", fmt.Sprintf("must be %q", SchemaVersion))
	}
}

func validateID(path, value string, errs *ValidationErrors) {
	if !idPattern.MatchString(value) {
		errs.add(path, "must be 1-63 lowercase letters, digits, dots, underscores, or hyphens and start with a letter or digit")
	}
}

func validateRequired(path, value string, errs *ValidationErrors) {
	if strings.TrimSpace(value) == "" {
		errs.add(path, "must not be empty")
	}
}

func validateStringList(path string, values []string, required bool, errs *ValidationErrors) {
	if required && len(values) == 0 {
		errs.add(path, "must contain at least one item")
	}
	seen := make(map[string]struct{}, len(values))
	for i, value := range values {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		value = strings.TrimSpace(value)
		if value == "" {
			errs.add(itemPath, "must not be empty")
			continue
		}
		if _, ok := seen[value]; ok {
			errs.add(itemPath, "must not duplicate an earlier item")
		}
		seen[value] = struct{}{}
	}
}

func validationResult(errs ValidationErrors) error {
	if len(errs) == 0 {
		return nil
	}
	return errs
}
