package domain

import (
	"encoding/json"
	"fmt"
	pathpkg "path"
	"regexp"
	"strconv"
	"strings"

	"github.com/bocacorazon/hermoso/internal/digest"
)

type VerificationArtifact struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Path            string `json:"path"`
	ContentHash     string `json:"content_hash"`
	PublicationPath string `json:"publication_path,omitempty"`
}

type VerificationExecution struct {
	Command          []string `json:"command"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
	TimeoutSeconds   uint64   `json:"timeout_seconds"`
}

type VerificationOracle struct {
	Type     string `json:"type"`
	Expected string `json:"expected,omitempty"`
}

type PropertyPolicy struct {
	Seed       uint64 `json:"seed"`
	Iterations uint64 `json:"iterations"`
}

type RubricPolicy struct {
	Judge    string   `json:"judge"`
	Criteria []string `json:"criteria"`
}

type VerificationJudgment struct {
	ID                     string                `json:"id"`
	Title                  string                `json:"title"`
	Modality               string                `json:"modality"`
	RequirementIDs         []string              `json:"requirement_ids"`
	AcceptanceCriterionIDs []string              `json:"acceptance_criterion_ids"`
	SurfaceIDs             []string              `json:"surface_ids"`
	BusinessTermIDs        []string              `json:"business_term_ids,omitempty"`
	InvariantNodeIDs       []string              `json:"invariant_node_ids,omitempty"`
	ArtifactIDs            []string              `json:"artifact_ids,omitempty"`
	ScenarioIDs            []string              `json:"scenario_ids,omitempty"`
	Execution              VerificationExecution `json:"execution"`
	Oracle                 VerificationOracle    `json:"oracle"`
	RequiredEvidence       []string              `json:"required_evidence"`
	Property               *PropertyPolicy       `json:"property,omitempty"`
	Rubric                 *RubricPolicy         `json:"rubric,omitempty"`
}

type CoverageExclusion struct {
	TargetKind string `json:"target_kind"`
	TargetID   string `json:"target_id"`
	Rationale  string `json:"rationale"`
}

type VerificationAggregation struct {
	Strategy string `json:"strategy"`
}

type FeatureVerificationContract struct {
	SchemaVersion      string                  `json:"schema_version"`
	Context            ContextRef              `json:"context"`
	Producer           Producer                `json:"producer"`
	Revision           uint64                  `json:"revision"`
	FeatureDesign      ContractReference       `json:"feature_design"`
	BaseModel          ModelReference          `json:"base_model"`
	Artifacts          []VerificationArtifact  `json:"artifacts"`
	Judgments          []VerificationJudgment  `json:"judgments"`
	CoverageExclusions []CoverageExclusion     `json:"coverage_exclusions,omitempty"`
	Aggregation        VerificationAggregation `json:"aggregation"`
}

func DesignPackageHash(
	revision uint64,
	featureHash string,
	verificationHash string,
	artifactRootHash string,
	baseModel ModelReference,
) (string, error) {
	return digest.JSON(struct {
		SchemaVersion    string         `json:"schema_version"`
		Revision         uint64         `json:"revision"`
		FeatureHash      string         `json:"feature_hash"`
		VerificationHash string         `json:"verification_hash"`
		ArtifactRootHash string         `json:"artifact_root_hash"`
		BaseModel        ModelReference `json:"base_model"`
	}{
		SchemaVersion: SchemaVersion, Revision: revision, FeatureHash: featureHash,
		VerificationHash: verificationHash, ArtifactRootHash: artifactRootHash, BaseModel: baseModel,
	})
}

func (c FeatureVerificationContract) Validate() error {
	var errs ValidationErrors
	validateVersion(c.SchemaVersion, &errs)
	c.Context.validate("context", &errs)
	c.Producer.validate("producer", &errs)
	if c.Revision == 0 {
		errs.add("revision", "must be greater than zero")
	}
	c.FeatureDesign.validate("feature_design", &errs)
	if !c.FeatureDesign.Context.Equal(c.Context) {
		errs.add("feature_design.context", "must match verification contract context")
	}
	if c.FeatureDesign.Kind != "feature-design" {
		errs.add("feature_design.kind", "must be feature-design")
	}
	c.BaseModel.validate("base_model", &errs)

	artifacts := map[string]struct{}{}
	publicationPaths := map[string]struct{}{}
	for i, artifact := range c.Artifacts {
		itemPath := fmt.Sprintf("artifacts[%d]", i)
		validateID(itemPath+".id", artifact.ID, &errs)
		if _, ok := artifacts[artifact.ID]; ok {
			errs.add(itemPath+".id", "must be unique")
		}
		artifacts[artifact.ID] = struct{}{}
		if artifact.Kind != "gherkin" && artifact.Kind != "fixture" &&
			artifact.Kind != "probe" && artifact.Kind != "generated_test" {
			errs.add(itemPath+".kind", "must be gherkin, fixture, probe, or generated_test")
		}
		if !validModelRelativePath(artifact.Path) {
			errs.add(itemPath+".path", "must be a clean relative path")
		}
		if !hashPattern.MatchString(artifact.ContentHash) {
			errs.add(itemPath+".content_hash", "must use sha256:<64 lowercase hex characters>")
		}
		if artifact.Kind == "gherkin" {
			if !validModelRelativePath(artifact.PublicationPath) {
				errs.add(itemPath+".publication_path", "must be a clean relative path for Gherkin")
			}
			if _, ok := publicationPaths[artifact.PublicationPath]; ok {
				errs.add(itemPath+".publication_path", "must be unique")
			}
			publicationPaths[artifact.PublicationPath] = struct{}{}
		} else if artifact.PublicationPath != "" {
			errs.add(itemPath+".publication_path", "is only allowed for Gherkin artifacts")
		}
	}
	if len(c.Artifacts) == 0 {
		errs.add("artifacts", "must contain at least one verifier artifact")
	}

	judgments := map[string]struct{}{}
	scenarios := map[string]struct{}{}
	coveredTargets := map[string]struct{}{}
	for i, judgment := range c.Judgments {
		itemPath := fmt.Sprintf("judgments[%d]", i)
		validateID(itemPath+".id", judgment.ID, &errs)
		if _, ok := judgments[judgment.ID]; ok {
			errs.add(itemPath+".id", "must be unique")
		}
		judgments[judgment.ID] = struct{}{}
		validateRequired(itemPath+".title", judgment.Title, &errs)
		if judgment.Modality != "bdd" && judgment.Modality != "deterministic" &&
			judgment.Modality != "property" && judgment.Modality != "rubric" {
			errs.add(itemPath+".modality", "must be bdd, deterministic, property, or rubric")
		}
		validateRequiredIDs(itemPath+".requirement_ids", judgment.RequirementIDs, true, &errs)
		validateRequiredIDs(itemPath+".acceptance_criterion_ids", judgment.AcceptanceCriterionIDs, true, &errs)
		for _, requirementID := range judgment.RequirementIDs {
			coveredTargets["requirement:"+requirementID] = struct{}{}
		}
		for _, criterionID := range judgment.AcceptanceCriterionIDs {
			coveredTargets["acceptance_criterion:"+criterionID] = struct{}{}
		}
		validateRequiredIDs(itemPath+".surface_ids", judgment.SurfaceIDs, true, &errs)
		validateRequiredIDs(itemPath+".business_term_ids", judgment.BusinessTermIDs, false, &errs)
		validateModelIDList(itemPath+".invariant_node_ids", judgment.InvariantNodeIDs, &errs)
		validateRequiredIDs(itemPath+".artifact_ids", judgment.ArtifactIDs, false, &errs)
		for j, artifactID := range judgment.ArtifactIDs {
			if _, ok := artifacts[artifactID]; !ok {
				errs.add(fmt.Sprintf("%s.artifact_ids[%d]", itemPath, j), "references an unknown artifact")
			}
		}
		validateRequiredIDs(itemPath+".scenario_ids", judgment.ScenarioIDs, false, &errs)
		for j, scenarioID := range judgment.ScenarioIDs {
			if _, ok := scenarios[scenarioID]; ok {
				errs.add(fmt.Sprintf("%s.scenario_ids[%d]", itemPath, j), "must be globally unique")
			}
			scenarios[scenarioID] = struct{}{}
		}
		judgment.Execution.validate(itemPath+".execution", &errs)
		judgment.Oracle.validate(itemPath+".oracle", judgment.Modality, &errs)
		validateStringList(itemPath+".required_evidence", judgment.RequiredEvidence, true, &errs)
		switch judgment.Modality {
		case "bdd":
			if len(judgment.ScenarioIDs) == 0 {
				errs.add(itemPath+".scenario_ids", "must contain at least one scenario for BDD")
			}
			if len(judgment.ArtifactIDs) == 0 {
				errs.add(itemPath+".artifact_ids", "must reference a Gherkin artifact")
			}
			if judgment.Property != nil || judgment.Rubric != nil {
				errs.add(itemPath, "BDD judgments must not declare property or rubric policy")
			}
		case "property":
			if judgment.Property == nil {
				errs.add(itemPath+".property", "must be set for property judgments")
			} else if judgment.Property.Iterations == 0 || judgment.Property.Iterations > 10000 {
				errs.add(itemPath+".property.iterations", "must be between 1 and 10000")
			}
			if judgment.Rubric != nil {
				errs.add(itemPath+".rubric", "must be empty for property judgments")
			}
		case "rubric":
			if judgment.Rubric == nil {
				errs.add(itemPath+".rubric", "must be set for rubric judgments")
			} else {
				validateRequired(itemPath+".rubric.judge", judgment.Rubric.Judge, &errs)
				validateStringList(itemPath+".rubric.criteria", judgment.Rubric.Criteria, true, &errs)
			}
			if judgment.Property != nil {
				errs.add(itemPath+".property", "must be empty for rubric judgments")
			}
		default:
			if judgment.Property != nil || judgment.Rubric != nil {
				errs.add(itemPath, "deterministic judgments must not declare property or rubric policy")
			}
		}
	}
	if len(c.Judgments) == 0 {
		errs.add("judgments", "must contain at least one judgment")
	}
	exclusions := map[string]struct{}{}
	for i, exclusion := range c.CoverageExclusions {
		itemPath := fmt.Sprintf("coverage_exclusions[%d]", i)
		if exclusion.TargetKind != "requirement" && exclusion.TargetKind != "acceptance_criterion" {
			errs.add(itemPath+".target_kind", "must be requirement or acceptance_criterion")
		}
		validateID(itemPath+".target_id", exclusion.TargetID, &errs)
		validateRequired(itemPath+".rationale", exclusion.Rationale, &errs)
		key := exclusion.TargetKind + ":" + exclusion.TargetID
		if _, ok := exclusions[key]; ok {
			errs.add(itemPath, "duplicates an earlier exclusion")
		}
		if _, ok := coveredTargets[key]; ok {
			errs.add(itemPath, "must not exclude a target covered by a judgment")
		}
		exclusions[key] = struct{}{}
	}
	if c.Aggregation.Strategy != "all_required" {
		errs.add("aggregation.strategy", "must be all_required")
	}
	return validationResult(errs)
}

func (e VerificationExecution) validate(path string, errs *ValidationErrors) {
	if len(e.Command) == 0 {
		errs.add(path+".command", "must contain an executable and argv")
	}
	for i, value := range e.Command {
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			errs.add(fmt.Sprintf("%s.command[%d]", path, i), "must be a non-empty argv value without control characters")
		}
		if len(e.Command) > 1 {
			executable := strings.ToLower(pathpkg.Base(strings.ReplaceAll(e.Command[0], "\\", "/")))
			flag := strings.ToLower(e.Command[1])
			if ((executable == "sh" || executable == "bash" || executable == "zsh" ||
				executable == "dash" || executable == "fish") && flag == "-c") ||
				((executable == "cmd" || executable == "cmd.exe") && flag == "/c") ||
				(strings.HasPrefix(executable, "powershell") && (flag == "-command" || flag == "-c")) {
				errs.add(path+".command", "must execute argv directly rather than a shell command string")
			}
		}
	}
	if e.WorkingDirectory != "" && !validModelRelativePath(e.WorkingDirectory) {
		errs.add(path+".working_directory", "must be a clean relative path")
	}
	if e.TimeoutSeconds == 0 || e.TimeoutSeconds > 3600 {
		errs.add(path+".timeout_seconds", "must be between 1 and 3600")
	}
}

func (o VerificationOracle) validate(path, modality string, errs *ValidationErrors) {
	if o.Type != "exit_code" && o.Type != "json_path" && o.Type != "stdout_regex" &&
		o.Type != "file" && o.Type != "gherkin" && o.Type != "rubric" {
		errs.add(path+".type", "must be exit_code, json_path, stdout_regex, file, gherkin, or rubric")
	}
	if modality == "bdd" && o.Type != "gherkin" {
		errs.add(path+".type", "must be gherkin for BDD judgments")
	}
	if modality == "rubric" && o.Type != "rubric" {
		errs.add(path+".type", "must be rubric for rubric judgments")
	}
	if o.Type != "gherkin" && o.Type != "rubric" {
		validateRequired(path+".expected", o.Expected, errs)
	}
	switch o.Type {
	case "exit_code":
		if _, err := strconv.Atoi(o.Expected); err != nil {
			errs.add(path+".expected", "must be an integer exit code")
		}
	case "stdout_regex":
		if _, err := regexp.Compile(o.Expected); err != nil {
			errs.add(path+".expected", "must be a valid regular expression")
		}
	case "file":
		if !validModelRelativePath(o.Expected) {
			errs.add(path+".expected", "must be a clean relative evidence path")
		}
	case "json_path":
		jsonPath, expected, ok := strings.Cut(o.Expected, "=")
		if !ok || strings.TrimSpace(jsonPath) == "" {
			errs.add(path+".expected", "must use dot.path=<JSON value>")
		} else if !json.Valid([]byte(expected)) {
			errs.add(path+".expected", "must end with a valid JSON value")
		}
	}
}

func validateRequiredIDs(path string, values []string, required bool, errs *ValidationErrors) {
	if required && len(values) == 0 {
		errs.add(path, "must contain at least one ID")
	}
	seen := map[string]struct{}{}
	for i, value := range values {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		validateID(itemPath, value, errs)
		if _, ok := seen[value]; ok {
			errs.add(itemPath, "must be unique")
		}
		seen[value] = struct{}{}
	}
}
