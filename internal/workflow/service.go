package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bocacorazon/hermoso/internal/contracts"
	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/dispatch"
	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/model"
	"github.com/bocacorazon/hermoso/internal/repository"
	"github.com/bocacorazon/hermoso/internal/state"
	"github.com/bocacorazon/hermoso/internal/verification"
)

type Service struct {
	Store   state.Store
	Manager *repository.Manager
	Now     func() time.Time
}

type Profile struct {
	Phases struct {
		Integration struct {
			Profile  string   `json:"profile"`
			Skills   []string `json:"skills"`
			GoalMode string   `json:"goal_mode"`
			Tier     string   `json:"tier,omitempty"`
		} `json:"integration"`
	} `json:"phases"`
	Dispatch struct {
		Priority                    int      `json:"priority"`
		DefaultRuntimeBudgetSeconds uint64   `json:"default_runtime_budget_seconds"`
		Prepare                     []string `json:"prepare"`
		Execute                     []string `json:"execute"`
		Validate                    []string `json:"validate"`
	} `json:"dispatch"`
}

func Hash(value any) (string, error) {
	return digest.JSON(value)
}

func (s Service) PutDesign(ctx context.Context, execution domain.ContextRef, data []byte) (domain.Run, bool, error) {
	var design domain.FeatureDesign
	if err := decodeContract(contracts.FeatureDesign, data, execution, &design); err != nil {
		return domain.Run{}, false, err
	}
	if !design.Context.Equal(execution) {
		return domain.Run{}, false, errors.New("feature design context does not match command context")
	}
	snapshot, err := s.Store.ModelSnapshot(ctx, design.BaseModel.SnapshotID)
	if err != nil {
		return domain.Run{}, false, fmt.Errorf("load feature design repository model: %w", err)
	}
	if err := verification.ValidateDesignModel(design, snapshot); err != nil {
		return domain.Run{}, false, err
	}
	if err := ensureModelSnapshotFresh(ctx, execution, snapshot); err != nil {
		return domain.Run{}, false, err
	}
	hash, err := Hash(design)
	if err != nil {
		return domain.Run{}, false, err
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if run.Phase != domain.PhaseDesign && !(run.Phase == domain.PhaseConstruction && len(run.ConstructionRounds) == 0) {
			return fmt.Errorf("design cannot be changed in phase %s", run.Phase)
		}
		if run.Design != nil {
			if run.Design.Feature.Revision > design.Revision {
				return errors.New("design revision must not move backwards")
			}
			if run.Design.Feature.Revision == design.Revision {
				if run.Design.FeatureHash == hash {
					return nil
				}
				return errors.New("design content changed without incrementing revision")
			}
		}
		packageRevision := uint64(1)
		if run.Design != nil {
			packageRevision = run.Design.Revision + 1
		}
		run.Design = &domain.DesignState{Revision: packageRevision, Feature: design, FeatureHash: hash}
		run.ConstructionRounds = nil
		run.Phase = domain.PhaseDesign
		run.Status = domain.StatusAwaitingApproval
		run.Revision++
		run.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, changed, err
}

func (s Service) BeginDesign(ctx context.Context, execution domain.ContextRef) (domain.Run, bool, error) {
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if run.Phase != domain.PhaseDesign {
			return fmt.Errorf("design begin requires design phase, got %s", run.Phase)
		}
		if run.Status != domain.StatusPending {
			if run.Status == domain.StatusInProgress {
				return nil // already in progress — no-op
			}
			return fmt.Errorf("design begin requires pending status, got %s", run.Status)
		}
		if err := domain.ValidateTransition(run.Phase, run.Status, domain.PhaseDesign, domain.StatusInProgress); err != nil {
			return err
		}
		run.Status = domain.StatusInProgress
		run.Revision++
		run.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, changed, err
}

func (s Service) PutVerificationContract(
	ctx context.Context,
	execution domain.ContextRef,
	data []byte,
	assets map[string][]byte,
) (domain.Run, bool, error) {
	var contract domain.FeatureVerificationContract
	if err := decodeContract(contracts.FeatureVerificationContract, data, execution, &contract); err != nil {
		return domain.Run{}, false, err
	}
	current, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, false, err
	}
	if current.Phase != domain.PhaseDesign || current.Design == nil {
		return domain.Run{}, false, errors.New("verification contract requires a persisted feature design in the design phase")
	}
	snapshot, err := s.Store.ModelSnapshot(ctx, current.Design.Feature.BaseModel.SnapshotID)
	if err != nil {
		return domain.Run{}, false, fmt.Errorf("load verification repository model: %w", err)
	}
	if err := ensureModelSnapshotFresh(ctx, execution, snapshot); err != nil {
		return domain.Run{}, false, err
	}
	if err := verification.ValidateContract(
		contract, current.Design.Feature, current.Design.FeatureHash, snapshot, assets,
	); err != nil {
		return domain.Run{}, false, err
	}
	artifactRootHash, err := s.Store.SealArtifacts(ctx, execution, assets)
	if err != nil {
		return domain.Run{}, false, err
	}
	contractHash, err := Hash(contract)
	if err != nil {
		return domain.Run{}, false, err
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if run.Phase != domain.PhaseDesign || run.Design == nil {
			return errors.New("verification contract requires a persisted feature design in the design phase")
		}
		if run.Design.FeatureHash != current.Design.FeatureHash {
			return errors.New("feature design changed while verification assets were being sealed")
		}
		if run.Design.Verification != nil {
			if run.Design.Verification.Revision > contract.Revision {
				return errors.New("verification contract revision must not move backwards")
			}
			if run.Design.Verification.Revision == contract.Revision {
				if run.Design.VerificationHash == contractHash && run.Design.ArtifactRootHash == artifactRootHash {
					return nil
				}
				return errors.New("verification contract content changed without incrementing revision")
			}
		}
		packageRevision := run.Design.Revision + 1
		packageHash, err := domain.DesignPackageHash(
			packageRevision, run.Design.FeatureHash, contractHash,
			artifactRootHash, run.Design.Feature.BaseModel,
		)
		if err != nil {
			return err
		}
		run.Design.Revision = packageRevision
		run.Design.Verification = &contract
		run.Design.VerificationHash = contractHash
		run.Design.ArtifactRootHash = artifactRootHash
		run.Design.PackageHash = packageHash
		run.Design.Approval = nil
		run.Status = domain.StatusAwaitingApproval
		run.Revision++
		run.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, changed, err
}

func (s Service) ApproveDesign(ctx context.Context, execution domain.ContextRef, revision uint64, hash, actor, comment string) (domain.Run, bool, error) {
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if run.Design == nil {
			return errors.New("no persisted feature design")
		}
		if run.Design.Verification == nil || run.Design.PackageHash == "" {
			return errors.New("design package is incomplete without a verification contract")
		}
		if run.Design.Revision != revision || run.Design.PackageHash != hash {
			return errors.New("approval revision or hash does not match current design package")
		}
		if run.Design.Approval != nil {
			if run.Design.Approval.Revision == revision && run.Design.Approval.PackageHash == hash && run.Design.Approval.Actor == actor {
				return nil
			}
			return errors.New("current design package already has a different approval")
		}
		approval := domain.Approval{
			SchemaVersion: domain.SchemaVersion, Context: execution, Phase: domain.PhaseDesign,
			Revision: revision, PackageHash: hash, Actor: actor, ApprovedAt: s.Now().UTC(), Comment: comment,
		}
		if err := approval.ValidatePackage(domain.PhaseDesign, revision, hash); err != nil {
			return err
		}
		run.Design.Approval = &approval
		run.Phase = domain.PhaseConstruction
		run.Status = domain.StatusPending
		run.Revision++
		run.UpdatedAt = approval.ApprovedAt
		changed = true
		return nil
	})
	return run, changed, err
}

func (s Service) PutGraph(ctx context.Context, execution domain.ContextRef, data []byte) (domain.Run, bool, error) {
	var graph domain.WorkGraph
	if err := decodeContract(contracts.WorkGraph, data, execution, &graph); err != nil {
		return domain.Run{}, false, err
	}
	if !graph.Context.Equal(execution) {
		return domain.Run{}, false, errors.New("work graph context does not match command context")
	}
	hash, err := Hash(graph)
	if err != nil {
		return domain.Run{}, false, err
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if err := ensureConstructionMutable(*run); err != nil {
			return err
		}
		if run.Phase != domain.PhaseConstruction || run.Design == nil ||
			run.Design.Verification == nil || run.Design.Approval == nil {
			return errors.New("work graph requires the exact current design package approval")
		}
		if err := validateGraphDesign(graph, run.Design.Feature); err != nil {
			return err
		}
		construction := run.CurrentConstruction()
		if construction != nil {
			if len(construction.Items) != 0 && construction.Hash != hash {
				return errors.New("cannot replace a graph after construction preparation")
			}
			if construction.Graph.Revision > graph.Revision {
				return errors.New("work graph revision must not move backwards")
			}
			if construction.Graph.Revision == graph.Revision {
				if construction.Hash == hash {
					return nil
				}
				return errors.New("work graph content changed without incrementing revision")
			}
		}
		run.ConstructionRounds = []domain.ConstructionState{{
			Number: 1, Kind: "initial", SourceHash: run.Design.PackageHash,
			Graph: graph, Hash: hash,
		}}
		run.Revision++
		run.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, changed, err
}

func LoadProfile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read profile: %w", err)
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, fmt.Errorf("decode profile: %w", err)
	}
	if profile.Dispatch.DefaultRuntimeBudgetSeconds == 0 {
		return Profile{}, errors.New("profile default runtime budget must be greater than zero")
	}
	return profile, nil
}

func canonicalProfilePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve profile path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonicalize profile path: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect profile path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("profile path must identify a regular file")
	}
	return canonical, nil
}

func (s Service) Prepare(ctx context.Context, execution domain.ContextRef, profilePath string) (domain.Run, dispatch.Plan, bool, error) {
	if profilePath == "" {
		resolved, err := s.Store.ResolveProfile(ctx, execution)
		if err != nil {
			return domain.Run{}, dispatch.Plan{}, false, err
		}
		profilePath = resolved
	}
	profilePath, err := canonicalProfilePath(profilePath)
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, false, err
	}
	profile, err := LoadProfile(profilePath)
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, false, err
	}
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, false, err
	}
	construction := run.CurrentConstruction()
	if construction == nil || run.Design == nil ||
		run.Design.Verification == nil || run.Design.Approval == nil {
		return domain.Run{}, dispatch.Plan{}, false, errors.New("construction preparation requires approved design package and work graph")
	}
	if err := ensureConstructionMutable(run); err != nil {
		return domain.Run{}, dispatch.Plan{}, false, err
	}
	parentBranch := run.Design.Feature.BaseModel.SourceRevision
	feature, err := s.Manager.Feature(ctx, execution.RunID, execution.FeatureID, parentBranch)
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, false, err
	}
	workspaces := make(map[string]string, len(construction.Graph.Items))
	states := make([]domain.WorkState, 0, len(construction.Graph.Items))
	for _, item := range construction.Graph.Items {
		worktree, blocked, err := s.Manager.WorkItem(ctx, execution.RunID, execution.FeatureID, item.ID, []repository.Worktree{feature})
		if err != nil {
			if blocked != nil {
				_, _ = s.blockRun(ctx, execution, err.Error())
			}
			return domain.Run{}, dispatch.Plan{}, false, err
		}
		workspaces[item.ID] = worktree.Path
		states = append(states, domain.WorkState{ID: item.ID, Status: domain.WorkPending, Workspace: workspace(worktree)})
	}
	plan, err := dispatch.Compile(construction.Graph, dispatch.Config{
		Priority: profile.Dispatch.Priority, PreparedWorkspaces: workspaces,
		DefaultRuntimeBudgetSeconds: profile.Dispatch.DefaultRuntimeBudgetSeconds,
		LifecycleCommands: dispatch.LifecycleCommands{
			Prepare: profile.Dispatch.Prepare, Execute: profile.Dispatch.Execute, Validate: profile.Dispatch.Validate,
		},
		Design: &run.Design.Feature,
		Round:  construction.Number,
	})
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, false, err
	}
	changed := false
	run, err = s.Store.UpdateRun(ctx, execution, func(current *domain.Run) error {
		currentConstruction := current.CurrentConstruction()
		if currentConstruction == nil || currentConstruction.Hash != construction.Hash {
			return errors.New("construction graph changed during preparation")
		}
		if len(currentConstruction.Items) != 0 {
			return nil
		}
		currentConstruction.ProfilePath = profilePath
		currentConstruction.Feature = workspace(feature)
		currentConstruction.Items = states
		current.Status = domain.StatusInProgress
		current.Revision++
		current.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, plan, changed, err
}

func (s Service) Plan(ctx context.Context, execution domain.ContextRef) (domain.Run, dispatch.Plan, error) {
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, err
	}
	construction := run.CurrentConstruction()
	if construction == nil || len(construction.Items) == 0 {
		return domain.Run{}, dispatch.Plan{}, errors.New("construction is not prepared")
	}
	profile, err := LoadProfile(construction.ProfilePath)
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, err
	}
	workspaces := map[string]string{}
	for _, item := range construction.Items {
		workspaces[item.ID] = item.Workspace.Path
	}
	plan, err := dispatch.Compile(construction.Graph, dispatch.Config{
		Priority: profile.Dispatch.Priority, PreparedWorkspaces: workspaces,
		DefaultRuntimeBudgetSeconds: profile.Dispatch.DefaultRuntimeBudgetSeconds,
		LifecycleCommands: dispatch.LifecycleCommands{
			Prepare: profile.Dispatch.Prepare, Execute: profile.Dispatch.Execute, Validate: profile.Dispatch.Validate,
		},
		Design: &run.Design.Feature,
		Round:  construction.Number,
	})
	return run, plan, err
}

func (s Service) Ready(ctx context.Context, execution domain.ContextRef) ([]dispatch.Card, error) {
	run, plan, err := s.Plan(ctx, execution)
	if err != nil {
		return nil, err
	}
	statuses := map[string]domain.WorkStatus{}
	bound := map[string]bool{}
	construction := run.CurrentConstruction()
	for _, item := range construction.Items {
		statuses[item.ID] = item.Status
	}
	var roundBindings []domain.TaskBinding
	for _, binding := range run.TaskBindings {
		if binding.Round != construction.Number {
			continue
		}
		bound[binding.WorkItemID] = true
		roundBindings = append(roundBindings, binding)
	}
	var ready []dispatch.Card
	for _, card := range plan.Cards {
		if bound[card.Identity.WorkItemID] {
			continue
		}
		if statuses[card.Identity.WorkItemID] != domain.WorkPending {
			continue
		}
		parentsComplete := true
		for _, parent := range card.Parents {
			if statuses[parent.WorkItemID] != domain.WorkCompleted {
				parentsComplete = false
				break
			}
		}
		if !parentsComplete {
			continue
		}
		resolved, err := dispatch.ResolveParents(card, roundBindings)
		if errors.Is(err, dispatch.ErrMissingParentBinding) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ready = append(ready, resolved)
	}
	return ready, nil
}

func (s Service) BindTask(ctx context.Context, execution domain.ContextRef, itemID, taskID string) (domain.TaskBinding, bool, error) {
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.TaskBinding{}, false, err
	}
	item, _, err := findItem(run, itemID)
	if err != nil {
		return domain.TaskBinding{}, false, err
	}
	if err := ensureConstructionMutable(run); err != nil {
		return domain.TaskBinding{}, false, err
	}
	if item.Status != domain.WorkPending && item.Status != domain.WorkBlocked {
		return domain.TaskBinding{}, false, fmt.Errorf("work item %q is not bindable from %s", itemID, item.Status)
	}
	construction := run.CurrentConstruction()
	return s.Store.BindTask(ctx, execution, construction.Number, itemID, taskID, s.Now())
}

func (s Service) StartWork(ctx context.Context, execution domain.ContextRef, itemID string) (domain.Run, bool, error) {
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, false, err
	}
	if err := ensureConstructionMutable(run); err != nil {
		return domain.Run{}, false, err
	}
	item, graphItem, err := findItem(run, itemID)
	if err != nil {
		return domain.Run{}, false, err
	}
	if item.Status == domain.WorkStarted {
		return run, false, nil
	}
	if item.Status != domain.WorkPending && item.Status != domain.WorkBlocked {
		return domain.Run{}, false, fmt.Errorf("work item %q cannot start from %s", itemID, item.Status)
	}
	parents := []repository.Worktree{}
	construction := run.CurrentConstruction()
	if len(graphItem.Parents) == 0 {
		parents = append(parents, repoWorkspace(construction.Feature))
	} else {
		for _, parentID := range graphItem.Parents {
			parent, _, err := findItem(run, parentID)
			if err != nil {
				return domain.Run{}, false, err
			}
			if parent.Status != domain.WorkCompleted {
				return domain.Run{}, false, fmt.Errorf("parent %q is not completed", parentID)
			}
			parents = append(parents, repoWorkspace(parent.Workspace))
		}
	}
	if _, blocked, err := s.Manager.WorkItem(ctx, execution.RunID, execution.FeatureID, itemID, parents); err != nil {
		if blocked != nil {
			_, _ = s.blockRun(ctx, execution, err.Error())
		}
		return domain.Run{}, false, err
	}
	changed := false
	run, err = s.Store.UpdateRun(ctx, execution, func(current *domain.Run) error {
		target, _, err := findItem(*current, itemID)
		if err != nil {
			return err
		}
		if target.Status == domain.WorkStarted {
			return nil
		}
		now := s.Now().UTC()
		target.Status, target.Blocker, target.StartedAt = domain.WorkStarted, "", &now
		current.Status = domain.StatusInProgress
		current.CurrentConstruction().Blocker = ""
		current.Revision++
		current.UpdatedAt = now
		changed = true
		return nil
	})
	return run, changed, err
}

func (s Service) FinishWork(ctx context.Context, execution domain.ContextRef, itemID string, status domain.WorkStatus, evidence domain.Evidence, blocker string) (domain.Run, bool, error) {
	if status != domain.WorkCompleted && status != domain.WorkBlocked {
		return domain.Run{}, false, errors.New("work outcome must be completed or blocked")
	}
	if !evidence.Context.Equal(execution) {
		return domain.Run{}, false, errors.New("evidence context does not match command context")
	}
	current, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, false, err
	}
	if err := ensureConstructionMutable(current); err != nil {
		return domain.Run{}, false, err
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		item, _, err := findItem(*run, itemID)
		if err != nil {
			return err
		}
		if item.Status == status && containsEvidence(item.Evidence, evidence.ID) {
			return nil
		}
		if item.Status != domain.WorkStarted && item.Status != domain.WorkBlocked {
			return fmt.Errorf("work item %q cannot finish from %s", itemID, item.Status)
		}
		if status == domain.WorkBlocked && strings.TrimSpace(blocker) == "" {
			return errors.New("blocked work requires a blocker")
		}
		now := s.Now().UTC()
		item.Status, item.Blocker, item.EndedAt = status, blocker, &now
		if !containsEvidence(item.Evidence, evidence.ID) {
			item.Evidence = append(item.Evidence, evidence)
			run.Evidence = append(run.Evidence, evidence)
		}
		if status == domain.WorkBlocked {
			run.Status = domain.StatusBlocked
			run.CurrentConstruction().Blocker = blocker
		}
		run.Revision++
		run.UpdatedAt = now
		changed = true
		return nil
	})
	return run, changed, err
}

func (s Service) PutResult(ctx context.Context, execution domain.ContextRef, data []byte) (domain.Run, bool, error) {
	var result domain.PhaseResult
	if err := decodeContract(contracts.PhaseResult, data, execution, &result); err != nil {
		return domain.Run{}, false, err
	}
	if !result.Context.Equal(execution) || result.Phase != domain.PhaseConstruction {
		return domain.Run{}, false, errors.New("construction result context or phase does not match")
	}
	hash, _ := Hash(result)
	if result.Status == domain.ResultCompleted {
		run, err := s.Store.Run(ctx, execution)
		if err != nil {
			return domain.Run{}, false, err
		}
		if err := s.validateIntegratedCommits(run); err != nil {
			return domain.Run{}, false, err
		}
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		construction := run.CurrentConstruction()
		if construction == nil {
			return errors.New("construction is not prepared")
		}
		if construction.Result != nil {
			existing, _ := Hash(*construction.Result)
			if existing == hash {
				return nil
			}
			return errors.New("a different construction result is already persisted")
		}
		if result.Status == domain.ResultCompleted {
			for _, item := range construction.Items {
				if item.Status != domain.WorkCompleted {
					return fmt.Errorf("work item %q is not completed", item.ID)
				}
			}
			if construction.IntegratedAt == nil {
				return errors.New("construction branches have not been integrated")
			}
			run.Status = domain.StatusAwaitingVerification
			construction.Blocker = ""
		} else if result.Status == domain.ResultBlocked {
			run.Status = domain.StatusBlocked
			construction.Blocker = strings.Join(result.Blockers, "; ")
		} else {
			return fmt.Errorf("construction result status %q is not accepted", result.Status)
		}
		construction.Result = &result
		for _, evidence := range result.Evidence {
			if !containsEvidence(run.Evidence, evidence.ID) {
				run.Evidence = append(run.Evidence, evidence)
			}
		}
		run.Revision++
		run.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, changed, err
}

func (s Service) Resume(ctx context.Context, execution domain.ContextRef) (domain.Run, bool, error) {
	existing, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, false, err
	}
	if existing.Phase == domain.PhaseVerification && existing.Status == domain.StatusBlocked &&
		existing.Publication == nil && len(existing.VerificationAttempts) > 0 {
		last := existing.VerificationAttempts[len(existing.VerificationAttempts)-1]
		if last.Report.Verdict == domain.VerificationPass {
			published, err := s.publishPassingGherkin(ctx, execution, existing, last)
			return published, err == nil, err
		}
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if run.Phase == domain.PhaseVerification && run.Status == domain.StatusBlocked {
			if len(run.VerificationAttempts) == 0 {
				return errors.New("blocked verification has no persisted attempt")
			}
			last := run.VerificationAttempts[len(run.VerificationAttempts)-1]
			if last.Report.Verdict != domain.VerificationBlocked &&
				last.Report.Verdict != domain.VerificationInconclusive {
				return errors.New("failed verification requires human action and cannot be resumed")
			}
			run.VerificationIncidents = append(run.VerificationIncidents, last)
			run.VerificationAttempts = run.VerificationAttempts[:len(run.VerificationAttempts)-1]
			run.VerificationBlocker = ""
			run.Phase = domain.PhaseConstruction
			run.Status = domain.StatusAwaitingVerification
			run.Revision++
			run.UpdatedAt = s.Now().UTC()
			changed = true
			return nil
		}
		if err := ensureConstructionMutable(*run); err != nil {
			return err
		}
		if run.Status != domain.StatusBlocked {
			return nil
		}
		construction := run.CurrentConstruction()
		if construction == nil {
			return errors.New("blocked run has no construction state to resume")
		}
		for i := range construction.Items {
			if construction.Items[i].Status == domain.WorkBlocked {
				construction.Items[i].Status = domain.WorkPending
				construction.Items[i].Blocker = ""
			}
		}
		construction.Blocker = ""
		construction.Result = nil
		run.Status = domain.StatusInProgress
		run.Revision++
		run.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, changed, err
}

func (s Service) Integrate(ctx context.Context, execution domain.ContextRef, checks []string) (domain.Run, repository.IntegrationResult, bool, error) {
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, repository.IntegrationResult{}, false, err
	}
	construction := run.CurrentConstruction()
	if construction == nil {
		return domain.Run{}, repository.IntegrationResult{}, false, errors.New("construction is not prepared")
	}
	if err := ensureConstructionMutable(run); err != nil {
		return domain.Run{}, repository.IntegrationResult{}, false, err
	}
	leaves, err := constructionLeaves(run)
	if err != nil {
		return domain.Run{}, repository.IntegrationResult{}, false, err
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].ID < leaves[j].ID })
	if construction.IntegratedAt != nil {
		if err := s.validateIntegratedCommits(run); err != nil {
			return domain.Run{}, repository.IntegrationResult{}, false, err
		}
	}
	result, integrateErr := s.Manager.Integrate(
		ctx, execution.RunID, execution.FeatureID, repoWorkspace(construction.Feature), leaves, checks,
	)
	changed := false
	if integrateErr != nil {
		run, _ = s.blockRun(ctx, execution, integrateErr.Error())
		changed = true
	} else {
		featureCommit, commitErr := s.Manager.CurrentCommit(repoWorkspace(construction.Feature))
		if commitErr != nil {
			return domain.Run{}, result, false, commitErr
		}
		integratedLeaves := make([]domain.IntegratedCommit, len(leaves))
		for i, leaf := range leaves {
			commit, commitErr := s.Manager.CurrentCommit(leaf)
			if commitErr != nil {
				return domain.Run{}, result, false, commitErr
			}
			integratedLeaves[i] = domain.IntegratedCommit{ID: leaf.ID, Branch: leaf.Branch, Commit: commit}
		}
		run, err = s.Store.UpdateRun(ctx, execution, func(current *domain.Run) error {
			currentConstruction := current.CurrentConstruction()
			if currentConstruction.IntegratedAt != nil {
				return nil
			}
			now := s.Now().UTC()
			currentConstruction.IntegratedAt = &now
			currentConstruction.IntegratedFeatureCommit = featureCommit
			currentConstruction.IntegratedLeaves = integratedLeaves
			currentConstruction.Blocker = ""
			current.Status = domain.StatusInProgress
			current.Revision++
			current.UpdatedAt = now
			changed = true
			return nil
		})
		if err != nil {
			return domain.Run{}, result, changed, err
		}
	}
	return run, result, changed, integrateErr
}

func ensureConstructionMutable(run domain.Run) error {
	construction := run.LatestConstruction()
	if run.Status == domain.StatusAwaitingVerification ||
		(construction != nil && construction.Result != nil && construction.Result.Status == domain.ResultCompleted) {
		return errors.New("completed construction is immutable")
	}
	return nil
}

func validateGraphDesign(graph domain.WorkGraph, design domain.FeatureDesign) error {
	requirements := make(map[string]struct{}, len(design.Requirements))
	criteria := make(map[string]domain.AcceptanceCriterion, len(design.AcceptanceCriteria))
	surfaces := make(map[string]struct{}, len(design.Surfaces))
	for _, requirement := range design.Requirements {
		requirements[requirement.ID] = struct{}{}
	}
	for _, criterion := range design.AcceptanceCriteria {
		criteria[criterion.ID] = criterion
	}
	for _, surface := range design.Surfaces {
		surfaces[surface.ID] = struct{}{}
	}
	coveredRequirements := map[string]struct{}{}
	coveredCriteria := map[string]struct{}{}
	coveredSurfaces := map[string]struct{}{}
	var problems []string
	for _, item := range graph.Items {
		itemRequirements := make(map[string]struct{}, len(item.RequirementIDs))
		for _, id := range item.RequirementIDs {
			if _, ok := requirements[id]; !ok {
				problems = append(problems, fmt.Sprintf("work item %q references unknown requirement %q", item.ID, id))
				continue
			}
			itemRequirements[id] = struct{}{}
			coveredRequirements[id] = struct{}{}
		}
		for _, id := range item.CriterionIDs {
			criterion, ok := criteria[id]
			if !ok {
				problems = append(problems, fmt.Sprintf("work item %q references unknown acceptance criterion %q", item.ID, id))
				continue
			}
			coveredCriteria[id] = struct{}{}
			for _, requirementID := range criterion.RequirementIDs {
				if _, ok := itemRequirements[requirementID]; !ok {
					problems = append(
						problems,
						fmt.Sprintf(
							"work item %q criterion %q requires requirement %q",
							item.ID, id, requirementID,
						),
					)
				}
			}
		}
		for _, id := range item.SurfaceIDs {
			if _, ok := surfaces[id]; !ok {
				problems = append(problems, fmt.Sprintf("work item %q references unknown surface %q", item.ID, id))
				continue
			}
			coveredSurfaces[id] = struct{}{}
		}
	}
	for id := range requirements {
		if _, ok := coveredRequirements[id]; !ok {
			problems = append(problems, fmt.Sprintf("requirement %q is not covered by the work graph", id))
		}
	}
	for id := range criteria {
		if _, ok := coveredCriteria[id]; !ok {
			problems = append(problems, fmt.Sprintf("acceptance criterion %q is not covered by the work graph", id))
		}
	}
	for id := range surfaces {
		if _, ok := coveredSurfaces[id]; !ok {
			problems = append(problems, fmt.Sprintf("surface %q is not covered by the work graph", id))
		}
	}
	if len(problems) != 0 {
		sort.Strings(problems)
		return fmt.Errorf("work graph design traceability failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

func ensureModelSnapshotFresh(
	ctx context.Context,
	execution domain.ContextRef,
	snapshot domain.ModelSnapshot,
) error {
	head, err := model.ResolveRevision(ctx, execution.Repository.Repository, "HEAD")
	if err != nil {
		return err
	}
	if snapshot.Manifest.SourceRevision != head {
		return fmt.Errorf(
			"repository model snapshot is stale: indexed %s, repository HEAD is %s",
			snapshot.Manifest.SourceRevision, head,
		)
	}
	return nil
}

func constructionLeaves(run domain.Run) ([]repository.Worktree, error) {
	construction := run.LatestConstruction()
	if construction == nil {
		return nil, errors.New("construction is not prepared")
	}
	children := map[string]bool{}
	for _, item := range construction.Graph.Items {
		for _, parent := range item.Parents {
			children[parent] = true
		}
	}
	var leaves []repository.Worktree
	for _, item := range construction.Items {
		if children[item.ID] {
			continue
		}
		if item.Status != domain.WorkCompleted {
			return nil, fmt.Errorf("leaf %q is not completed", item.ID)
		}
		leaves = append(leaves, repoWorkspace(item.Workspace))
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].ID < leaves[j].ID })
	return leaves, nil
}

func (s Service) validateIntegratedCommits(run domain.Run) error {
	construction := run.LatestConstruction()
	if construction == nil || construction.IntegratedAt == nil {
		return errors.New("construction branches have not been integrated")
	}
	featureCommit, err := s.Manager.CurrentCommit(repoWorkspace(construction.Feature))
	if err != nil {
		return err
	}
	if featureCommit != construction.IntegratedFeatureCommit {
		return errors.New("integrated feature commit has drifted")
	}
	leaves, err := constructionLeaves(run)
	if err != nil {
		return err
	}
	if len(leaves) != len(construction.IntegratedLeaves) {
		return errors.New("integrated leaf set has drifted")
	}
	for i, leaf := range leaves {
		recorded := construction.IntegratedLeaves[i]
		if recorded.ID != leaf.ID || recorded.Branch != leaf.Branch {
			return errors.New("integrated leaf identity has drifted")
		}
		commit, err := s.Manager.CurrentCommit(leaf)
		if err != nil {
			return err
		}
		if commit != recorded.Commit {
			return fmt.Errorf("integrated leaf %q commit has drifted", leaf.ID)
		}
	}
	return nil
}

func (s Service) blockRun(ctx context.Context, execution domain.ContextRef, blocker string) (domain.Run, error) {
	return s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		run.Status = domain.StatusBlocked
		if construction := run.CurrentConstruction(); construction != nil {
			construction.Blocker = blocker
		}
		run.Revision++
		run.UpdatedAt = s.Now().UTC()
		return nil
	})
}

func findItem(run domain.Run, id string) (*domain.WorkState, *domain.WorkItem, error) {
	construction := run.CurrentConstruction()
	if construction == nil {
		return nil, nil, errors.New("construction is not prepared")
	}
	var stateItem *domain.WorkState
	for i := range construction.Items {
		if construction.Items[i].ID == id {
			stateItem = &construction.Items[i]
			break
		}
	}
	var graphItem *domain.WorkItem
	for i := range construction.Graph.Items {
		if construction.Graph.Items[i].ID == id {
			graphItem = &construction.Graph.Items[i]
			break
		}
	}
	if stateItem == nil || graphItem == nil {
		return nil, nil, fmt.Errorf("unknown work item %q", id)
	}
	return stateItem, graphItem, nil
}

func workspace(value repository.Worktree) domain.Workspace {
	return domain.Workspace{
		Kind: value.Kind, ID: value.ID, Branch: value.Branch, Path: value.Path,
		ParentBranch: value.ParentBranch, ParentCommit: value.ParentCommit,
	}
}

func repoWorkspace(value domain.Workspace) repository.Worktree {
	return repository.Worktree{
		Kind: value.Kind, ID: value.ID, Branch: value.Branch, Path: value.Path,
		ParentBranch: value.ParentBranch, ParentCommit: value.ParentCommit,
	}
}

func containsEvidence(items []domain.Evidence, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func decodeContract(kind contracts.Kind, data []byte, execution domain.ContextRef, target any) error {
	if err := contracts.ValidateForContext(kind, data, execution); err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", kind, err)
	}
	return nil
}

// Release transitions a run from awaiting_release through the release phase
// to released. It first moves the run into the release phase (pending → in_progress),
// runs any provided check commands in the feature worktree, and then transitions
// to released on success or blocked on check failure.
func (s Service) Release(
	ctx context.Context,
	execution domain.ContextRef,
	checks []string,
) (domain.Run, repository.IntegrationResult, error) {
	run, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, repository.IntegrationResult{}, err
	}
	if run.Phase != domain.PhaseVerification || run.Status != domain.StatusAwaitingRelease {
		return domain.Run{}, repository.IntegrationResult{}, errors.New("release requires a run in awaiting_release state")
	}
	if run.Publication == nil {
		return domain.Run{}, repository.IntegrationResult{}, errors.New("release requires published Gherkin")
	}

	// Transition: Verification/AwaitingRelease → Release/Pending
	run, err = s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
		if r.Phase != domain.PhaseVerification || r.Status != domain.StatusAwaitingRelease {
			return errors.New("run is no longer awaiting release")
		}
		r.Phase = domain.PhaseRelease
		r.Status = domain.StatusPending
		r.Revision++
		r.UpdatedAt = s.Now().UTC()
		return nil
	})
	if err != nil {
		return run, repository.IntegrationResult{}, err
	}

	// Transition: Release/Pending → Release/InProgress
	run, err = s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
		if r.Phase != domain.PhaseRelease || r.Status != domain.StatusPending {
			return errors.New("run is no longer in release/pending")
		}
		r.Phase = domain.PhaseRelease
		r.Status = domain.StatusInProgress
		r.Revision++
		r.UpdatedAt = s.Now().UTC()
		return nil
	})
	if err != nil {
		return run, repository.IntegrationResult{}, err
	}

	// Run optional checks in the feature worktree.
	result := repository.IntegrationResult{}
	construction := run.CurrentConstruction()
	if construction == nil {
		return run, result, errors.New("release requires a construction with a feature workspace")
	}
	feature := repoWorkspace(construction.Feature)
	if len(checks) > 0 {
		checkResults, checkErr := s.Manager.RunChecks(ctx, feature, checks)
		result.Checks = checkResults
		if checkErr != nil {
			// Transition: Release/InProgress → Release/Blocked
			run, _ = s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
				if r.Phase != domain.PhaseRelease || r.Status != domain.StatusInProgress {
					return nil
				}
				r.Phase = domain.PhaseRelease
				r.Status = domain.StatusBlocked
				r.Revision++
				r.UpdatedAt = s.Now().UTC()
				return nil
			})
			return run, result, checkErr
		}
	}

	// Merge the feature branch into the repository's default branch (e.g. main).
	defaultBranch := execution.Repository.DefaultBranch
	if defaultBranch == "" {
		return run, result, errors.New("release requires a default branch in the project context")
	}
	mergeCommit, mergeErr := s.Manager.MergeToDefault(ctx, feature, defaultBranch)
	if mergeErr != nil {
		// Transition: Release/InProgress → Release/Blocked
		run, _ = s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
			if r.Phase != domain.PhaseRelease || r.Status != domain.StatusInProgress {
				return nil
			}
			r.Phase = domain.PhaseRelease
			r.Status = domain.StatusBlocked
			r.Revision++
			r.UpdatedAt = s.Now().UTC()
			return nil
		})
		return run, result, mergeErr
	}

	// Transition: Release/InProgress → Release/Released
	run, err = s.Store.UpdateRun(ctx, execution, func(r *domain.Run) error {
		if r.Phase != domain.PhaseRelease || r.Status != domain.StatusInProgress {
			return errors.New("run is no longer in release/in_progress")
		}
		r.Phase = domain.PhaseRelease
		r.Status = domain.StatusReleased
		r.ReleaseCommit = mergeCommit
		r.Revision++
		r.UpdatedAt = s.Now().UTC()
		return nil
	})
	return run, result, err
}

