package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/model"
)

func (s Service) RunVerification(
	ctx context.Context,
	execution domain.ContextRef,
) (domain.Run, domain.VerificationReport, error) {
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	construction := run.LatestConstruction()
	if run.Phase != domain.PhaseConstruction || run.Status != domain.StatusAwaitingVerification ||
		construction == nil || construction.Result == nil ||
		construction.Result.Status != domain.ResultCompleted {
		return domain.Run{}, domain.VerificationReport{}, errors.New("verification requires completed integrated construction")
	}
	if run.Design == nil || run.Design.Verification == nil || run.Design.Approval == nil {
		return domain.Run{}, domain.VerificationReport{}, errors.New("verification requires an approved design package")
	}
	if len(run.VerificationAttempts) >= 2 {
		return domain.Run{}, domain.VerificationReport{}, errors.New("verification is limited to two attempts")
	}
	if err := s.validateIntegratedCommits(run); err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	attemptNumber := uint64(len(run.VerificationAttempts) + 1)
	candidateCommit := construction.IntegratedFeatureCommit
	startedAt := s.Now().UTC()
	verificationWorkspace, err := s.Manager.Verification(
		ctx, execution.RunID, execution.FeatureID, attemptNumber, candidateCommit,
	)
	if err != nil {
		return s.persistBlockedVerification(
			ctx, run, attemptNumber, candidateCommit, domain.Workspace{}, run.Design.Feature.BaseModel,
			"prepare isolated verification workspace: "+err.Error(),
		)
	}
	if commit, err := s.Manager.CurrentCommit(verificationWorkspace); err != nil {
		return s.persistBlockedVerification(
			ctx, run, attemptNumber, candidateCommit, workspace(verificationWorkspace),
			run.Design.Feature.BaseModel, "resolve verification workspace commit: "+err.Error(),
		)
	} else if commit != candidateCommit {
		return s.persistBlockedVerification(
			ctx, run, attemptNumber, candidateCommit, workspace(verificationWorkspace),
			run.Design.Feature.BaseModel, "verification workspace is not at the integrated candidate commit",
		)
	}
	assetRoot, err := s.Store.MaterializeVerificationArtifacts(
		ctx, execution, attemptNumber, run.Design.ArtifactRootHash,
		run.Design.Verification.Artifacts,
	)
	if err != nil {
		return s.persistBlockedVerification(
			ctx, run, attemptNumber, candidateCommit, workspace(verificationWorkspace),
			run.Design.Feature.BaseModel, "materialize sealed verifier assets: "+err.Error(),
		)
	}
	project, err := s.Store.Project(ctx)
	if err != nil {
		return s.persistBlockedVerification(
			ctx, run, attemptNumber, candidateCommit, workspace(verificationWorkspace),
			run.Design.Feature.BaseModel, "load project identity: "+err.Error(),
		)
	}
	candidateSnapshot, err := model.Build(ctx, model.BuildRequest{
		Project: project, RepositoryRoot: project.Target.Repository,
		Revision: candidateCommit, GeneratedAt: startedAt,
	})
	if err != nil {
		return s.persistBlockedVerification(
			ctx, run, attemptNumber, candidateCommit, workspace(verificationWorkspace),
			run.Design.Feature.BaseModel, "build candidate repository model: "+err.Error(),
		)
	}
	candidateSnapshot, _, err = s.Store.PutModelSnapshot(ctx, candidateSnapshot)
	if err != nil {
		return s.persistBlockedVerification(
			ctx, run, attemptNumber, candidateCommit, workspace(verificationWorkspace),
			modelReference(candidateSnapshot), "persist candidate repository model: "+err.Error(),
		)
	}
	candidateModel := modelReference(candidateSnapshot)
	resolutions := resolveCandidateSurfaces(run.Design.Feature, candidateSnapshot)
	before, err := worktreeFingerprint(verificationWorkspace.Path)
	if err != nil {
		return s.persistBlockedVerification(
			ctx, run, attemptNumber, candidateCommit, workspace(verificationWorkspace),
			candidateModel, "fingerprint candidate worktree: "+err.Error(),
		)
	}
	outcomes := make([]domain.JudgmentOutcome, 0, len(run.Design.Verification.Judgments))
	for _, judgment := range run.Design.Verification.Judgments {
		outcomes = append(outcomes, executeJudgment(ctx, verificationWorkspace.Path, assetRoot, judgment))
	}
	after, fingerprintErr := worktreeFingerprint(verificationWorkspace.Path)
	commitAfter, commitErr := s.Manager.CurrentCommit(verificationWorkspace)
	findings := []string{}
	if fingerprintErr != nil {
		findings = append(findings, "candidate mutation check failed: "+fingerprintErr.Error())
	} else if before != after {
		findings = append(findings, "verification mutated the candidate worktree")
	}
	if commitErr != nil {
		findings = append(findings, "candidate commit check failed: "+commitErr.Error())
	} else if commitAfter != candidateCommit {
		findings = append(findings, "verification changed the candidate commit")
	}
	missingSurfaces := map[string]struct{}{}
	for _, resolution := range resolutions {
		if resolution.Status == "missing" {
			missingSurfaces[resolution.SurfaceID] = struct{}{}
			findings = append(findings, resolution.Summary)
		}
	}
	for i := range outcomes {
		for _, surfaceID := range outcomes[i].SurfaceIDs {
			if _, missing := missingSurfaces[surfaceID]; missing && outcomes[i].Status == domain.JudgmentPass {
				outcomes[i].Status = domain.JudgmentFail
				outcomes[i].Summary = "candidate interaction surface did not resolve"
			}
		}
	}
	verdict := aggregateVerdict(outcomes)
	// Pending surface resolutions cause the verdict to be pending — the skill
	// must resolve them before the verdict can be final.
	for _, resolution := range resolutions {
		if resolution.Status == "pending" && verdict != domain.VerificationBlocked {
			verdict = domain.VerificationPending
			break
		}
	}
	if fingerprintErr != nil || commitErr != nil || before != after || commitAfter != candidateCommit {
		verdict = domain.VerificationBlocked
	}
	report := domain.VerificationReport{
		SchemaVersion: domain.SchemaVersion, Context: execution,
		Producer: domain.Producer{Skill: "hermoso-verification", Runtime: "deterministic"},
		Attempt:  attemptNumber, CandidateCommit: candidateCommit,
		DesignPackageHash: run.Design.PackageHash, CandidateModel: candidateModel,
		Outcomes: outcomes,
		Requirements: aggregateCoverage(outcomes, func(outcome domain.JudgmentOutcome) []string {
			return outcome.RequirementIDs
		}, run.Design.Verification.CoverageExclusions, "requirement"),
		AcceptanceCriteria: aggregateCoverage(outcomes, func(outcome domain.JudgmentOutcome) []string {
			return outcome.AcceptanceCriterionIDs
		}, run.Design.Verification.CoverageExclusions, "acceptance_criterion"),
		SurfaceResolutions: resolutions, Findings: findings,
		Verdict: verdict, StartedAt: startedAt, CompletedAt: s.Now().UTC(),
	}
	if err := validateReportCompleteness(report, *run.Design.Verification); err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	reportHash, err := Hash(report)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	attempt := domain.VerificationAttempt{
		Number: attemptNumber, CandidateCommit: candidateCommit,
		PackageHash: run.Design.PackageHash, ArtifactRootHash: run.Design.ArtifactRootHash,
		BaseModel: run.Design.Feature.BaseModel, CandidateModel: candidateModel,
		Workspace: workspace(verificationWorkspace), Report: report, ReportHash: reportHash,
	}
	run, err = s.persistVerificationAttempt(ctx, execution, attempt)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	if verdict == domain.VerificationPass {
		run, err = s.publishPassingGherkin(ctx, execution, run, attempt)
		if err != nil {
			blocked, blockErr := s.Store.UpdateRun(ctx, execution, func(current *domain.Run) error {
				current.Phase = domain.PhaseVerification
				current.Status = domain.StatusBlocked
				current.VerificationBlocker = "Gherkin publication failed: " + err.Error()
				current.Revision++
				current.UpdatedAt = s.Now().UTC()
				return nil
			})
			if blockErr != nil {
				return domain.Run{}, report, errors.Join(err, blockErr)
			}
			return blocked, report, err
		}
	}
	return run, report, err
}

func (s Service) persistBlockedVerification(
	ctx context.Context,
	run domain.Run,
	attemptNumber uint64,
	candidateCommit string,
	verificationWorkspace domain.Workspace,
	candidateModel domain.ModelReference,
	finding string,
) (domain.Run, domain.VerificationReport, error) {
	now := s.Now().UTC()
	outcomes := make([]domain.JudgmentOutcome, 0, len(run.Design.Verification.Judgments))
	for _, judgment := range run.Design.Verification.Judgments {
		outcomes = append(outcomes, domain.JudgmentOutcome{
			JudgmentID: judgment.ID, Status: domain.JudgmentBlocked,
			RequirementIDs:         append([]string(nil), judgment.RequirementIDs...),
			AcceptanceCriterionIDs: append([]string(nil), judgment.AcceptanceCriterionIDs...),
			SurfaceIDs:             append([]string(nil), judgment.SurfaceIDs...),
			Summary:                "verification could not execute",
			Evidence: domain.VerificationEvidence{
				ExitCode: -1, StdoutHash: digest.Bytes(nil), StderrHash: digest.Bytes([]byte(finding)),
			},
		})
	}
	report := domain.VerificationReport{
		SchemaVersion: domain.SchemaVersion, Context: run.Context,
		Producer: domain.Producer{Skill: "hermoso-verification", Runtime: "deterministic"},
		Attempt:  attemptNumber, CandidateCommit: candidateCommit,
		DesignPackageHash: run.Design.PackageHash, CandidateModel: candidateModel,
		Outcomes: outcomes,
		Requirements: aggregateCoverage(outcomes, func(outcome domain.JudgmentOutcome) []string {
			return outcome.RequirementIDs
		}, run.Design.Verification.CoverageExclusions, "requirement"),
		AcceptanceCriteria: aggregateCoverage(outcomes, func(outcome domain.JudgmentOutcome) []string {
			return outcome.AcceptanceCriterionIDs
		}, run.Design.Verification.CoverageExclusions, "acceptance_criterion"),
		Findings: []string{finding}, Verdict: domain.VerificationBlocked,
		StartedAt: now, CompletedAt: now,
	}
	if err := validateReportCompleteness(report, *run.Design.Verification); err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	reportHash, err := Hash(report)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	persisted, err := s.persistVerificationAttempt(ctx, run.Context, domain.VerificationAttempt{
		Number: attemptNumber, CandidateCommit: candidateCommit,
		PackageHash: run.Design.PackageHash, ArtifactRootHash: run.Design.ArtifactRootHash,
		BaseModel: run.Design.Feature.BaseModel, CandidateModel: candidateModel,
		Workspace: verificationWorkspace, Report: report, ReportHash: reportHash,
	})
	return persisted, report, err
}

func (s Service) persistVerificationAttempt(
	ctx context.Context,
	execution domain.ContextRef,
	attempt domain.VerificationAttempt,
) (domain.Run, error) {
	return s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if uint64(len(run.VerificationAttempts)+1) != attempt.Number {
			return errors.New("verification attempt order changed while verification was running")
		}
		if run.Design == nil || run.Design.PackageHash != attempt.PackageHash ||
			run.Design.ArtifactRootHash != attempt.ArtifactRootHash {
			return errors.New("design package changed while verification was running")
		}
		run.VerificationAttempts = append(run.VerificationAttempts, attempt)
		now := s.Now().UTC()
		switch attempt.Report.Verdict {
		case domain.VerificationPass:
			run.Phase = domain.PhaseVerification
			run.Status = domain.StatusInProgress
		case domain.VerificationPending:
			run.Phase = domain.PhaseVerification
			run.Status = domain.StatusAwaitingJudgment
		case domain.VerificationFail:
			if attempt.Number == 1 {
				run.Phase = domain.PhaseConstruction
				run.Status = domain.StatusAwaitingRemediation
			} else {
				run.Phase = domain.PhaseVerification
				run.Status = domain.StatusBlocked
			}
		default:
			run.Phase = domain.PhaseVerification
			run.Status = domain.StatusBlocked
		}
		run.Revision++
		run.UpdatedAt = now
		return nil
	})
}

func executeJudgment(
	ctx context.Context,
	workspaceRoot, assetRoot string,
	judgment domain.VerificationJudgment,
) domain.JudgmentOutcome {
	started := time.Now()
	timeout, cancel := context.WithTimeout(ctx, time.Duration(judgment.Execution.TimeoutSeconds)*time.Second)
	defer cancel()
	workingDirectory := workspaceRoot
	if judgment.Execution.WorkingDirectory != "" {
		workingDirectory = filepath.Join(workspaceRoot, filepath.FromSlash(judgment.Execution.WorkingDirectory))
	}
	command := exec.CommandContext(timeout, judgment.Execution.Command[0], judgment.Execution.Command[1:]...)
	command.Dir = workingDirectory
	command.Env = append(
		os.Environ(),
		"HERMOSO_VERIFICATION_ASSETS="+assetRoot,
	)
	if judgment.Property != nil {
		command.Env = append(
			command.Env,
			fmt.Sprintf("HERMOSO_PROPERTY_SEED=%d", judgment.Property.Seed),
			fmt.Sprintf("HERMOSO_PROPERTY_ITERATIONS=%d", judgment.Property.Iterations),
		)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	runErr := command.Run()
	exitCode := 0
	if runErr != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	status, summary := judgeExecution(timeout, judgment, workingDirectory, stdout.Bytes(), exitCode, runErr)
	evidence := domain.VerificationEvidence{
		Command:          append([]string(nil), judgment.Execution.Command...),
		WorkingDirectory: workingDirectory, ExitCode: exitCode,
		Stdout: stdout.String(), Stderr: stderr.String(),
		StdoutHash: digest.Bytes(stdout.Bytes()), StderrHash: digest.Bytes(stderr.Bytes()),
		DurationMillis: time.Since(started).Milliseconds(),
	}
	if judgment.Property != nil {
		seed, iterations := judgment.Property.Seed, judgment.Property.Iterations
		evidence.Seed, evidence.Iterations = &seed, &iterations
	}
	if judgment.Rubric != nil {
		evidence.Judge = judgment.Rubric.Judge
	}
	return domain.JudgmentOutcome{
		JudgmentID: judgment.ID, Status: status,
		RequirementIDs:         append([]string(nil), judgment.RequirementIDs...),
		AcceptanceCriterionIDs: append([]string(nil), judgment.AcceptanceCriterionIDs...),
		SurfaceIDs:             append([]string(nil), judgment.SurfaceIDs...),
		Summary:                summary, Evidence: evidence,
	}
}

func judgeExecution(
	ctx context.Context,
	judgment domain.VerificationJudgment,
	workingDirectory string,
	stdout []byte,
	exitCode int,
	runErr error,
) (domain.JudgmentStatus, string) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return domain.JudgmentBlocked, "verification command timed out"
	}
	if runErr != nil && exitCode == -1 {
		return domain.JudgmentBlocked, "verification command could not start: " + runErr.Error()
	}
	switch judgment.Oracle.Type {
	case "rubric":
		return domain.JudgmentPending, "rubric oracle requires qualitative judgment by the verification skill"
	case "gherkin":
		if exitCode == 0 {
			return domain.JudgmentPass, "approved external judge passed"
		}
	case "exit_code":
		expected, err := strconv.Atoi(judgment.Oracle.Expected)
		if err != nil {
			return domain.JudgmentBlocked, "exit-code oracle is invalid"
		}
		if exitCode == expected {
			return domain.JudgmentPass, fmt.Sprintf("exit code matched %d", expected)
		}
	case "stdout_regex":
		pattern, err := regexp.Compile(judgment.Oracle.Expected)
		if err != nil {
			return domain.JudgmentBlocked, "stdout oracle regular expression is invalid"
		}
		if pattern.Match(stdout) {
			return domain.JudgmentPass, "stdout matched the approved oracle"
		}
	case "file":
		path := filepath.Join(workingDirectory, filepath.FromSlash(judgment.Oracle.Expected))
		if _, err := os.Stat(path); err == nil && exitCode == 0 {
			return domain.JudgmentPass, "approved evidence file exists"
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return domain.JudgmentBlocked, "file oracle could not inspect evidence"
		}
	case "json_path":
		var value any
		if json.Unmarshal(stdout, &value) != nil {
			return domain.JudgmentFail, "stdout was not valid JSON"
		}
		jsonPath, expectedJSON, _ := strings.Cut(judgment.Oracle.Expected, "=")
		var expected any
		if json.Unmarshal([]byte(expectedJSON), &expected) != nil {
			return domain.JudgmentBlocked, "JSON-path oracle expected value is invalid"
		}
		actual, ok := lookupJSONPath(value, jsonPath)
		if exitCode == 0 && ok && reflect.DeepEqual(actual, expected) {
			return domain.JudgmentPass, "JSON path matched the approved value"
		}
	}
	return domain.JudgmentFail, fmt.Sprintf("approved oracle was not satisfied (exit code %d)", exitCode)
}

func lookupJSONPath(value any, jsonPath string) (any, bool) {
	jsonPath = strings.TrimPrefix(strings.TrimSpace(jsonPath), "$.")
	current := value
	for _, segment := range strings.Split(jsonPath, ".") {
		if segment == "" {
			return nil, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func resolveCandidateSurfaces(
	design domain.FeatureDesign,
	snapshot domain.ModelSnapshot,
) []domain.SurfaceResolution {
	nodes := make(map[string]domain.ModelNode, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		nodes[node.ID] = node
	}
	resolutions := make([]domain.SurfaceResolution, 0, len(design.Surfaces))
	for _, surface := range design.Surfaces {
		resolution := domain.SurfaceResolution{SurfaceID: surface.ID}
		if surface.Source == "existing" {
			node, ok := nodes[surface.ModelNodeID]
			if ok && node.Kind == "interface" && node.Attributes["interface_kind"] == surface.Kind {
				resolution.ModelNodeID, resolution.Status = node.ID, "resolved"
				resolution.Summary = "existing interaction surface resolved"
			} else {
				resolution.Status = "missing"
				resolution.Summary = fmt.Sprintf("existing surface %q is missing or contradictory", surface.ID)
			}
		} else {
			// Planned surfaces require skill-authored resolution — Go does not
			// attempt semantic matching. Check for cooperative surface_id tagging
			// (deterministic), otherwise mark as pending for the skill to resolve.
			for _, node := range snapshot.Nodes {
				if node.Kind == "interface" && node.Attributes["interface_kind"] == surface.Kind &&
					node.Attributes["surface_id"] == surface.ID {
					resolution.ModelNodeID, resolution.Status = node.ID, "resolved"
					resolution.Summary = "planned interaction surface resolved via cooperative tagging"
					break
				}
			}
			if resolution.Status == "" {
				resolution.Status = "pending"
				resolution.Summary = fmt.Sprintf("planned surface %q requires skill resolution", surface.ID)
			}
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions
}

func aggregateVerdict(outcomes []domain.JudgmentOutcome) domain.VerificationVerdict {
	verdict := domain.VerificationPass
	for _, outcome := range outcomes {
		if outcome.Status == domain.JudgmentBlocked {
			return domain.VerificationBlocked
		}
		if outcome.Status == domain.JudgmentPending {
			verdict = domain.VerificationPending
			continue
		}
		if outcome.Status == domain.JudgmentFail && verdict != domain.VerificationPending {
			verdict = domain.VerificationFail
		}
	}
	return verdict
}

func aggregateCoverage(
	outcomes []domain.JudgmentOutcome,
	ids func(domain.JudgmentOutcome) []string,
	exclusions []domain.CoverageExclusion,
	targetKind string,
) []domain.CoverageOutcome {
	statuses := map[string]domain.JudgmentStatus{}
	for _, outcome := range outcomes {
		for _, id := range ids(outcome) {
			current, exists := statuses[id]
			switch {
			case !exists:
				statuses[id] = outcome.Status
			case current == domain.JudgmentBlocked || outcome.Status == domain.JudgmentBlocked:
				statuses[id] = domain.JudgmentBlocked
			case current == domain.JudgmentFail || outcome.Status == domain.JudgmentFail:
				statuses[id] = domain.JudgmentFail
			default:
				statuses[id] = domain.JudgmentPass
			}
		}
	}
	result := make([]domain.CoverageOutcome, 0, len(statuses))
	for id, status := range statuses {
		result = append(result, domain.CoverageOutcome{ID: id, Status: string(status)})
	}
	for _, exclusion := range exclusions {
		if exclusion.TargetKind == targetKind {
			result = append(result, domain.CoverageOutcome{
				ID: exclusion.TargetID, Status: "excluded", Rationale: exclusion.Rationale,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func validateReportCompleteness(
	report domain.VerificationReport,
	contract domain.FeatureVerificationContract,
) error {
	if err := report.Validate(); err != nil {
		return err
	}
	expected := make(map[string]struct{}, len(contract.Judgments))
	for _, judgment := range contract.Judgments {
		expected[judgment.ID] = struct{}{}
	}
	for _, outcome := range report.Outcomes {
		if _, ok := expected[outcome.JudgmentID]; !ok {
			return fmt.Errorf("verification report contains unknown judgment %q", outcome.JudgmentID)
		}
		delete(expected, outcome.JudgmentID)
	}
	if len(expected) != 0 {
		return errors.New("verification report omitted required judgments")
	}
	expectedRequirements := map[string]struct{}{}
	expectedCriteria := map[string]struct{}{}
	for _, judgment := range contract.Judgments {
		for _, id := range judgment.RequirementIDs {
			expectedRequirements[id] = struct{}{}
		}
		for _, id := range judgment.AcceptanceCriterionIDs {
			expectedCriteria[id] = struct{}{}
		}
	}
	for _, exclusion := range contract.CoverageExclusions {
		if exclusion.TargetKind == "requirement" {
			expectedRequirements[exclusion.TargetID] = struct{}{}
		} else {
			expectedCriteria[exclusion.TargetID] = struct{}{}
		}
	}
	for _, outcome := range report.Requirements {
		delete(expectedRequirements, outcome.ID)
	}
	for _, outcome := range report.AcceptanceCriteria {
		delete(expectedCriteria, outcome.ID)
	}
	if len(expectedRequirements) != 0 || len(expectedCriteria) != 0 {
		return errors.New("verification report omitted requirement or acceptance-criterion coverage")
	}
	return nil
}

func (s Service) PutSurfaceResolutions(
	ctx context.Context,
	execution domain.ContextRef,
	resolutions []domain.SurfaceResolution,
) (domain.Run, domain.VerificationReport, error) {
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	if run.Phase != domain.PhaseVerification || run.Status != domain.StatusAwaitingJudgment {
		return domain.Run{}, domain.VerificationReport{}, errors.New("surface resolution requires a run in awaiting_judgment state")
	}
	attempt := &run.VerificationAttempts[len(run.VerificationAttempts)-1]
	report := attempt.Report

	// Build a set of submitted surface IDs
	submitted := map[string]domain.SurfaceResolution{}
	for _, res := range resolutions {
		submitted[res.SurfaceID] = res
	}

	// Apply submitted resolutions to pending entries
	pendingCount := 0
	for i, res := range report.SurfaceResolutions {
		if res.Status == "pending" {
			pendingCount++
			if update, ok := submitted[res.SurfaceID]; ok {
				report.SurfaceResolutions[i] = update
				pendingCount--
			}
		}
	}
	if pendingCount > 0 {
		return domain.Run{}, domain.VerificationReport{}, errors.New("not all pending surface resolutions were provided")
	}

	// Apply the invariant: unresolved surfaces fail linked judgments
	missingSurfaces := map[string]struct{}{}
	for _, res := range report.SurfaceResolutions {
		if res.Status != "resolved" {
			missingSurfaces[res.SurfaceID] = struct{}{}
		}
	}
	for i := range report.Outcomes {
		for _, surfaceID := range report.Outcomes[i].SurfaceIDs {
			if _, missing := missingSurfaces[surfaceID]; missing && report.Outcomes[i].Status == domain.JudgmentPass {
				report.Outcomes[i].Status = domain.JudgmentFail
				report.Outcomes[i].Summary = "candidate interaction surface did not resolve"
			}
		}
	}

	// Re-aggregate verdict
	verdict := aggregateVerdict(report.Outcomes)
	for _, res := range report.SurfaceResolutions {
		if res.Status == "pending" && verdict != domain.VerificationBlocked {
			verdict = domain.VerificationPending
			break
		}
	}
	report.Verdict = verdict

	if err := report.Validate(); err != nil {
		return domain.Run{}, domain.VerificationReport{}, fmt.Errorf("judgeVerification: report validation failed: %w", err)
	}

	reportHash, err := Hash(report)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	attempt.Report = report
	attempt.ReportHash = reportHash

	// Sync candidate model source revision for non-pending, non-blocked verdicts
	if report.Verdict != domain.VerificationPending && report.Verdict != domain.VerificationBlocked {
		report.CandidateModel.SourceRevision = report.CandidateCommit
		attempt.Report = report
		attempt.CandidateModel = report.CandidateModel
	}

	updatedRun, err := s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
		r.VerificationAttempts[len(r.VerificationAttempts)-1] = *attempt
		switch report.Verdict {
		case domain.VerificationPass:
			r.Phase = domain.PhaseVerification
			r.Status = domain.StatusInProgress
		case domain.VerificationFail:
			if attempt.Number == 1 {
				r.Phase = domain.PhaseConstruction
				r.Status = domain.StatusAwaitingRemediation
			} else {
				r.Phase = domain.PhaseVerification
				r.Status = domain.StatusBlocked
			}
		case domain.VerificationPending:
			r.Phase = domain.PhaseVerification
			r.Status = domain.StatusAwaitingJudgment
		default:
			r.Phase = domain.PhaseVerification
			r.Status = domain.StatusBlocked
		}
		r.Revision++
		r.UpdatedAt = s.Now().UTC()
		return nil
	})
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	// On pass, run the publication flow — same as RunVerification and JudgeVerification.
	if report.Verdict == domain.VerificationPass {
		hasGherkin := false
		for _, artifact := range updatedRun.Design.Verification.Artifacts {
			if artifact.Kind == "gherkin" {
				hasGherkin = true
				break
			}
		}
		if hasGherkin {
			updatedRun, err = s.publishPassingGherkin(ctx, execution, updatedRun, *attempt)
			if err != nil {
				blocked, blockErr := s.Store.UpdateRun(ctx, execution, func(current *domain.Run) error {
					current.Phase = domain.PhaseVerification
					current.Status = domain.StatusBlocked
					current.VerificationBlocker = "Gherkin publication failed: " + err.Error()
					current.Revision++
					current.UpdatedAt = s.Now().UTC()
					return nil
				})
				if blockErr != nil {
					return domain.Run{}, report, errors.Join(err, blockErr)
				}
				return blocked, report, err
			}
		} else {
			updatedRun, err = s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
				r.Phase = domain.PhaseVerification
				r.Status = domain.StatusAwaitingRelease
				r.VerificationBlocker = ""
				r.Revision++
				r.UpdatedAt = s.Now().UTC()
				return nil
			})
			if err != nil {
				return domain.Run{}, report, err
			}
		}
	}
	return updatedRun, report, err
}

func (s Service) PutRemediation(
	ctx context.Context,
	execution domain.ContextRef,
	data []byte,
) (domain.Run, bool, error) {
	var spec domain.SkillRemediation
	if err := json.Unmarshal(data, &spec); err != nil {
		return domain.Run{}, false, fmt.Errorf("decode skill remediation: %w", err)
	}
	if len(spec.Needs) == 0 {
		return domain.Run{}, false, errors.New("remediation spec must contain at least one need")
	}
	if !spec.Graph.Context.Equal(execution) {
		return domain.Run{}, false, errors.New("remediation graph context does not match command context")
	}
	if err := spec.Graph.Validate(); err != nil {
		return domain.Run{}, false, fmt.Errorf("remediation graph validation: %w", err)
	}
	graphHash, err := Hash(spec.Graph)
	if err != nil {
		return domain.Run{}, false, err
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
		if r.Phase != domain.PhaseConstruction || r.Status != domain.StatusAwaitingRemediation {
			return errors.New("remediation requires a run in awaiting_remediation state")
		}
		if len(r.VerificationAttempts) == 0 {
			return errors.New("no verification attempt to remediate")
		}
		attempt := r.VerificationAttempts[len(r.VerificationAttempts)-1]
		remediationSpec := domain.RemediationSpec{
			SchemaVersion:    domain.SchemaVersion,
			Context:          r.Context,
			FailedReportHash: attempt.ReportHash,
			Needs:            spec.Needs,
			CreatedAt:        attempt.Report.CompletedAt,
		}
		round := domain.ConstructionState{
			Number:      uint64(len(r.ConstructionRounds) + 1),
			Kind:        "remediation",
			SourceHash:  attempt.ReportHash,
			Remediation: &remediationSpec,
			Graph:       spec.Graph,
			Hash:        graphHash,
		}
		r.ConstructionRounds = append(r.ConstructionRounds, round)
		r.Phase = domain.PhaseConstruction
		r.Status = domain.StatusPending
		r.Revision++
		r.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, changed, err
}

func worktreeFingerprint(root string) (string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if relative == "." || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported candidate worktree entry %q", relative)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = fmt.Sprintf("%o:%s", info.Mode().Perm(), digest.Bytes(data))
		return nil
	})
	if err != nil {
		return "", err
	}
	return digest.JSON(files)
}

func modelReference(snapshot domain.ModelSnapshot) domain.ModelReference {
	return domain.ModelReference{
		SchemaVersion: snapshot.Manifest.SchemaVersion,
		SnapshotID:    snapshot.Manifest.SnapshotID, ContentHash: snapshot.Manifest.ContentHash,
		SourceRevision:    snapshot.Manifest.SourceRevision,
		VocabularyVersion: snapshot.Manifest.VocabularyVersion,
	}
}

// JudgeVerification is Phase 2 of the two-phase verification flow.
// The verification skill calls this to submit qualitative judgments for
// outcomes that were left pending by the mechanical Phase 1 run (rubric oracles).
// Go validates that every pending outcome receives a judgment, applies the
// judgments to the report, re-aggregates the verdict, and transitions the run.
func (s Service) JudgeVerification(
	ctx context.Context,
	execution domain.ContextRef,
	judgments []domain.SkillJudgment,
) (domain.Run, domain.VerificationReport, error) {
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	if run.Status != domain.StatusAwaitingJudgment {
		return domain.Run{}, domain.VerificationReport{}, fmt.Errorf(
			"judgeVerification: run status is %s, must be awaiting_judgment", run.Status,
		)
	}
	if len(run.VerificationAttempts) == 0 {
		return domain.Run{}, domain.VerificationReport{}, errors.New("judgeVerification: no verification attempts")
	}
	attempt := &run.VerificationAttempts[len(run.VerificationAttempts)-1]
	report := attempt.Report

	// Build a lookup of submitted judgments by ID.
	submitted := make(map[string]domain.SkillJudgment, len(judgments))
	for _, j := range judgments {
		submitted[j.JudgmentID] = j
	}

	// Validate: every pending outcome must have a corresponding judgment,
	// and no judgment may target a non-pending outcome.
	pendingIDs := make(map[string]bool)
	for i := range report.Outcomes {
		outcome := &report.Outcomes[i]
		if outcome.Status == domain.JudgmentPending {
			pendingIDs[outcome.JudgmentID] = true
			j, ok := submitted[outcome.JudgmentID]
			if !ok {
				return domain.Run{}, domain.VerificationReport{}, fmt.Errorf(
					"judgeVerification: pending outcome %q has no submitted judgment", outcome.JudgmentID,
				)
			}
			if j.Status != domain.JudgmentPass && j.Status != domain.JudgmentFail {
				return domain.Run{}, domain.VerificationReport{}, fmt.Errorf(
					"judgeVerification: judgment %q has invalid status %q (must be pass or fail)",
					j.JudgmentID, j.Status,
				)
			}
			if j.Qualitative == nil {
				return domain.Run{}, domain.VerificationReport{}, fmt.Errorf(
					"judgeVerification: judgment %q is missing qualitative details", j.JudgmentID,
				)
			}
		}
	}
	for _, j := range judgments {
		if !pendingIDs[j.JudgmentID] {
			return domain.Run{}, domain.VerificationReport{}, fmt.Errorf(
				"judgeVerification: judgment %q does not correspond to a pending outcome", j.JudgmentID,
			)
		}
	}
	if len(pendingIDs) == 0 {
		return domain.Run{}, domain.VerificationReport{}, errors.New(
			"judgeVerification: no pending outcomes to judge",
		)
	}

	// Apply judgments to outcomes.
	for i := range report.Outcomes {
		outcome := &report.Outcomes[i]
		if outcome.Status != domain.JudgmentPending {
			continue
		}
		j := submitted[outcome.JudgmentID]
		outcome.Status = j.Status
		outcome.Summary = j.Summary
		outcome.QualitativeJudgment = j.Qualitative
	}

	// Re-aggregate verdict and coverage.
	// After judgment, the verification is complete — update the candidate model
	// source revision to match the candidate commit (required for non-pending verdicts).
	report.CandidateModel.SourceRevision = report.CandidateCommit
	attempt.CandidateModel.SourceRevision = report.CandidateCommit
	report.Verdict = aggregateVerdict(report.Outcomes)
	report.Requirements = aggregateCoverage(report.Outcomes,
		func(o domain.JudgmentOutcome) []string { return o.RequirementIDs }, nil, "requirement")
	report.AcceptanceCriteria = aggregateCoverage(report.Outcomes,
		func(o domain.JudgmentOutcome) []string { return o.AcceptanceCriterionIDs }, nil, "acceptance_criterion")

	if err := report.Validate(); err != nil {
		return domain.Run{}, domain.VerificationReport{}, fmt.Errorf("judgeVerification: report validation failed: %w", err)
	}

	reportHash, err := Hash(report)
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}
	attempt.Report = report
	attempt.ReportHash = reportHash

	// Persist: update the attempt in-place and transition the run.
	updatedRun, err := s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
		r.VerificationAttempts[len(r.VerificationAttempts)-1] = *attempt
		switch report.Verdict {
		case domain.VerificationPass:
			r.Phase = domain.PhaseVerification
			r.Status = domain.StatusInProgress
		case domain.VerificationFail:
			if attempt.Number == 1 {
				r.Phase = domain.PhaseConstruction
				r.Status = domain.StatusAwaitingRemediation
			} else {
				r.Phase = domain.PhaseVerification
				r.Status = domain.StatusBlocked
			}
		case domain.VerificationPending:
			r.Phase = domain.PhaseVerification
			r.Status = domain.StatusAwaitingJudgment
		default:
			r.Phase = domain.PhaseVerification
			r.Status = domain.StatusBlocked
		}
		r.Revision++
		r.UpdatedAt = s.Now().UTC()
		return nil
	})
	if err != nil {
		return domain.Run{}, domain.VerificationReport{}, err
	}

	// On pass, run the publication flow — same as RunVerification.
	if report.Verdict == domain.VerificationPass {
		hasGherkin := false
		for _, artifact := range updatedRun.Design.Verification.Artifacts {
			if artifact.Kind == "gherkin" {
				hasGherkin = true
				break
			}
		}
		if hasGherkin {
			updatedRun, err = s.publishPassingGherkin(ctx, execution, updatedRun, *attempt)
			if err != nil {
				blocked, blockErr := s.Store.UpdateRun(ctx, execution, func(current *domain.Run) error {
					current.Phase = domain.PhaseVerification
					current.Status = domain.StatusBlocked
					current.VerificationBlocker = "Gherkin publication failed: " + err.Error()
					current.Revision++
					current.UpdatedAt = s.Now().UTC()
					return nil
				})
				if blockErr != nil {
					return domain.Run{}, report, errors.Join(err, blockErr)
				}
				return blocked, report, err
			}
		} else {
			// No gherkin artifacts to publish — transition directly to awaiting_release.
			updatedRun, err = s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
				r.Phase = domain.PhaseVerification
				r.Status = domain.StatusAwaitingRelease
				r.VerificationBlocker = ""
				r.Revision++
				r.UpdatedAt = s.Now().UTC()
				return nil
			})
			if err != nil {
				return domain.Run{}, report, err
			}
		}
	}
	return updatedRun, report, nil
}
