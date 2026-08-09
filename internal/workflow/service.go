package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bocacorazon/hermoso/internal/contracts"
	"github.com/bocacorazon/hermoso/internal/dispatch"
	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/repository"
	"github.com/bocacorazon/hermoso/internal/state"
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
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (s Service) PutDesign(ctx context.Context, execution domain.ContextRef, data []byte) (domain.Run, bool, error) {
	var design domain.FeatureDesign
	if err := decodeContract(contracts.FeatureDesign, data, execution, &design); err != nil {
		return domain.Run{}, false, err
	}
	if !design.Context.Equal(execution) {
		return domain.Run{}, false, errors.New("feature design context does not match command context")
	}
	hash, err := Hash(design)
	if err != nil {
		return domain.Run{}, false, err
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if run.Phase != domain.PhaseDesign && !(run.Phase == domain.PhaseConstruction && run.Construction == nil) {
			return fmt.Errorf("design cannot be changed in phase %s", run.Phase)
		}
		if run.Design != nil {
			if run.Design.Contract.Revision > design.Revision {
				return errors.New("design revision must not move backwards")
			}
			if run.Design.Contract.Revision == design.Revision {
				if run.Design.Hash == hash {
					return nil
				}
				return errors.New("design content changed without incrementing revision")
			}
		}
		run.Design = &domain.DesignState{Contract: design, Hash: hash}
		run.Construction = nil
		run.Phase = domain.PhaseDesign
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
		if run.Design.Contract.Revision != revision || run.Design.Hash != hash {
			return errors.New("approval revision or hash does not match current design")
		}
		if run.Design.Approval != nil {
			if run.Design.Approval.Revision == revision && run.Design.Approval.ContractHash == hash && run.Design.Approval.Actor == actor {
				return nil
			}
			return errors.New("current design already has a different approval")
		}
		approval := domain.Approval{
			SchemaVersion: domain.SchemaVersion, Context: execution, Phase: domain.PhaseDesign,
			Revision: revision, ContractHash: hash, Actor: actor, ApprovedAt: s.Now().UTC(), Comment: comment,
		}
		if err := approval.ValidateContract(domain.PhaseDesign, revision, hash); err != nil {
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
		if run.Phase != domain.PhaseConstruction || run.Design == nil || run.Design.Approval == nil {
			return errors.New("work graph requires the exact current design approval")
		}
		if run.Construction != nil {
			if len(run.Construction.Items) != 0 && run.Construction.Hash != hash {
				return errors.New("cannot replace a graph after construction preparation")
			}
			if run.Construction.Graph.Revision > graph.Revision {
				return errors.New("work graph revision must not move backwards")
			}
			if run.Construction.Graph.Revision == graph.Revision {
				if run.Construction.Hash == hash {
					return nil
				}
				return errors.New("work graph content changed without incrementing revision")
			}
		}
		run.Construction = &domain.ConstructionState{Graph: graph, Hash: hash}
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
	if run.Construction == nil || run.Design == nil || run.Design.Approval == nil {
		return domain.Run{}, dispatch.Plan{}, false, errors.New("construction preparation requires approved design and work graph")
	}
	if err := ensureConstructionMutable(run); err != nil {
		return domain.Run{}, dispatch.Plan{}, false, err
	}
	parentBranch := execution.Repository.DefaultBranch
	if parentBranch == "" {
		parentBranch = "HEAD"
	}
	feature, err := s.Manager.Feature(ctx, execution.RunID, execution.FeatureID, parentBranch)
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, false, err
	}
	workspaces := make(map[string]string, len(run.Construction.Graph.Items))
	states := make([]domain.WorkState, 0, len(run.Construction.Graph.Items))
	for _, item := range run.Construction.Graph.Items {
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
	plan, err := dispatch.Compile(run.Construction.Graph, dispatch.Config{
		Priority: profile.Dispatch.Priority, PreparedWorkspaces: workspaces,
		DefaultRuntimeBudgetSeconds: profile.Dispatch.DefaultRuntimeBudgetSeconds,
		LifecycleCommands: dispatch.LifecycleCommands{
			Prepare: profile.Dispatch.Prepare, Execute: profile.Dispatch.Execute, Validate: profile.Dispatch.Validate,
		},
	})
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, false, err
	}
	changed := false
	run, err = s.Store.UpdateRun(ctx, execution, func(current *domain.Run) error {
		if current.Construction == nil || current.Construction.Hash != run.Construction.Hash {
			return errors.New("construction graph changed during preparation")
		}
		if len(current.Construction.Items) != 0 {
			return nil
		}
		current.Construction.ProfilePath = profilePath
		current.Construction.Feature = workspace(feature)
		current.Construction.Items = states
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
	if run.Construction == nil || len(run.Construction.Items) == 0 {
		return domain.Run{}, dispatch.Plan{}, errors.New("construction is not prepared")
	}
	profile, err := LoadProfile(run.Construction.ProfilePath)
	if err != nil {
		return domain.Run{}, dispatch.Plan{}, err
	}
	workspaces := map[string]string{}
	for _, item := range run.Construction.Items {
		workspaces[item.ID] = item.Workspace.Path
	}
	plan, err := dispatch.Compile(run.Construction.Graph, dispatch.Config{
		Priority: profile.Dispatch.Priority, PreparedWorkspaces: workspaces,
		DefaultRuntimeBudgetSeconds: profile.Dispatch.DefaultRuntimeBudgetSeconds,
		LifecycleCommands: dispatch.LifecycleCommands{
			Prepare: profile.Dispatch.Prepare, Execute: profile.Dispatch.Execute, Validate: profile.Dispatch.Validate,
		},
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
	for _, item := range run.Construction.Items {
		statuses[item.ID] = item.Status
	}
	for _, binding := range run.TaskBindings {
		bound[binding.WorkItemID] = true
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
		resolved, err := dispatch.ResolveParents(card, run.TaskBindings)
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
	return s.Store.BindTask(ctx, execution, itemID, taskID, s.Now())
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
	if len(graphItem.Parents) == 0 {
		parents = append(parents, repoWorkspace(run.Construction.Feature))
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
		current.Construction.Blocker = ""
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
			run.Construction.Blocker = blocker
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
		if run.Construction == nil {
			return errors.New("construction is not prepared")
		}
		if run.Construction.Result != nil {
			existing, _ := Hash(*run.Construction.Result)
			if existing == hash {
				return nil
			}
			return errors.New("a different construction result is already persisted")
		}
		if result.Status == domain.ResultCompleted {
			for _, item := range run.Construction.Items {
				if item.Status != domain.WorkCompleted {
					return fmt.Errorf("work item %q is not completed", item.ID)
				}
			}
			if run.Construction.IntegratedAt == nil {
				return errors.New("construction branches have not been integrated")
			}
			run.Status = domain.StatusAwaitingVerification
			run.Construction.Blocker = ""
		} else if result.Status == domain.ResultBlocked {
			run.Status = domain.StatusBlocked
			run.Construction.Blocker = strings.Join(result.Blockers, "; ")
		} else {
			return fmt.Errorf("construction result status %q is not accepted", result.Status)
		}
		run.Construction.Result = &result
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
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if err := ensureConstructionMutable(*run); err != nil {
			return err
		}
		if run.Status != domain.StatusBlocked {
			return nil
		}
		if run.Construction == nil {
			return errors.New("blocked run has no construction state to resume")
		}
		for i := range run.Construction.Items {
			if run.Construction.Items[i].Status == domain.WorkBlocked {
				run.Construction.Items[i].Status = domain.WorkPending
				run.Construction.Items[i].Blocker = ""
			}
		}
		run.Construction.Blocker = ""
		run.Construction.Result = nil
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
	if run.Construction == nil {
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
	if run.Construction.IntegratedAt != nil {
		if err := s.validateIntegratedCommits(run); err != nil {
			return domain.Run{}, repository.IntegrationResult{}, false, err
		}
	}
	result, integrateErr := s.Manager.Integrate(ctx, execution.RunID, execution.FeatureID, repoWorkspace(run.Construction.Feature), leaves, checks)
	changed := false
	if integrateErr != nil {
		run, _ = s.blockRun(ctx, execution, integrateErr.Error())
		changed = true
	} else {
		featureCommit, commitErr := s.Manager.CurrentCommit(repoWorkspace(run.Construction.Feature))
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
			if current.Construction.IntegratedAt != nil {
				return nil
			}
			now := s.Now().UTC()
			current.Construction.IntegratedAt = &now
			current.Construction.IntegratedFeatureCommit = featureCommit
			current.Construction.IntegratedLeaves = integratedLeaves
			current.Construction.Blocker = ""
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
	if run.Status == domain.StatusAwaitingVerification ||
		(run.Construction != nil && run.Construction.Result != nil && run.Construction.Result.Status == domain.ResultCompleted) {
		return errors.New("completed construction is immutable")
	}
	return nil
}

func constructionLeaves(run domain.Run) ([]repository.Worktree, error) {
	children := map[string]bool{}
	for _, item := range run.Construction.Graph.Items {
		for _, parent := range item.Parents {
			children[parent] = true
		}
	}
	var leaves []repository.Worktree
	for _, item := range run.Construction.Items {
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
	if run.Construction == nil || run.Construction.IntegratedAt == nil {
		return errors.New("construction branches have not been integrated")
	}
	featureCommit, err := s.Manager.CurrentCommit(repoWorkspace(run.Construction.Feature))
	if err != nil {
		return err
	}
	if featureCommit != run.Construction.IntegratedFeatureCommit {
		return errors.New("integrated feature commit has drifted")
	}
	leaves, err := constructionLeaves(run)
	if err != nil {
		return err
	}
	if len(leaves) != len(run.Construction.IntegratedLeaves) {
		return errors.New("integrated leaf set has drifted")
	}
	for i, leaf := range leaves {
		recorded := run.Construction.IntegratedLeaves[i]
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
		if run.Construction != nil {
			run.Construction.Blocker = blocker
		}
		run.Revision++
		run.UpdatedAt = s.Now().UTC()
		return nil
	})
}

func findItem(run domain.Run, id string) (*domain.WorkState, *domain.WorkItem, error) {
	if run.Construction == nil {
		return nil, nil, errors.New("construction is not prepared")
	}
	var stateItem *domain.WorkState
	for i := range run.Construction.Items {
		if run.Construction.Items[i].ID == id {
			stateItem = &run.Construction.Items[i]
			break
		}
	}
	var graphItem *domain.WorkItem
	for i := range run.Construction.Graph.Items {
		if run.Construction.Graph.Items[i].ID == id {
			graphItem = &run.Construction.Graph.Items[i]
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
