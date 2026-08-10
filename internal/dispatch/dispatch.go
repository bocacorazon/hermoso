// Package dispatch compiles construction work graphs into runtime-neutral
// Kanban card specifications.
package dispatch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bocacorazon/hermoso/internal/domain"
)

const WorkspaceKindWorktree = "worktree"

var ErrMissingParentBinding = errors.New("missing parent task binding")

type LifecycleCommands struct {
	Prepare  []string `json:"prepare,omitempty"`
	Execute  []string `json:"execute,omitempty"`
	Validate []string `json:"validate,omitempty"`
}

type Config struct {
	Priority                    int
	PreparedWorkspaces          map[string]string
	DefaultRuntimeBudgetSeconds uint64
	LifecycleCommands           LifecycleCommands
	Integration                 *SyntheticIntegration
	Design                      *domain.FeatureDesign
	Round                       uint64
}

type SyntheticIntegration struct {
	ID                   string
	Title                string
	Body                 string
	AcceptanceCriteria   []string
	Worker               domain.Worker
	RuntimeBudgetSeconds uint64
	GoalMode             string
}

type WorkItemIdentity struct {
	Context    domain.ContextRef `json:"context"`
	Round      uint64            `json:"round"`
	WorkItemID string            `json:"work_item_id"`
	Synthetic  bool              `json:"synthetic,omitempty"`
}

type ParentReference struct {
	WorkItemID string `json:"work_item_id"`
	TaskID     string `json:"task_id,omitempty"`
}

type Card struct {
	Identity             WorkItemIdentity             `json:"identity"`
	Title                string                       `json:"title"`
	Body                 string                       `json:"body"`
	Parents              []ParentReference            `json:"parents,omitempty"`
	Tenant               string                       `json:"tenant"`
	Priority             int                          `json:"priority"`
	WorkspaceKind        string                       `json:"workspace_kind"`
	WorkspacePath        string                       `json:"workspace_path"`
	AssignedProfile      string                       `json:"assigned_profile"`
	ForcedSkills         []domain.SkillBinding        `json:"forced_skills"`
	RuntimeBudgetSeconds uint64                       `json:"runtime_budget_seconds"`
	GoalMode             string                       `json:"goal_mode,omitempty"`
	LifecycleCommands    LifecycleCommands            `json:"lifecycle_commands"`
	AcceptanceCriteria   []string                     `json:"acceptance_criteria"`
	Requirements         []domain.Requirement         `json:"requirements,omitempty"`
	DesignCriteria       []domain.AcceptanceCriterion `json:"design_acceptance_criteria,omitempty"`
	Surfaces             []domain.FeatureSurface      `json:"interaction_surfaces,omitempty"`
	Constraints          []string                     `json:"constraints,omitempty"`
	BaseModel            *domain.ModelReference       `json:"base_model,omitempty"`
	IdempotencyKey       string                       `json:"idempotency_key"`
}

type CardSpec = Card

type Plan struct {
	Context domain.ContextRef `json:"context"`
	Round   uint64            `json:"round"`
	Cards   []Card            `json:"cards"`
}

func Compile(graph domain.WorkGraph, config Config) (Plan, error) {
	if err := graph.Validate(); err != nil {
		return Plan{}, fmt.Errorf("validate work graph: %w", err)
	}
	if err := validateConfig(config); err != nil {
		return Plan{}, err
	}
	if config.Round == 0 {
		config.Round = 1
	}

	items := append([]domain.WorkItem(nil), graph.Items...)
	if config.Integration != nil {
		integration, err := integrationItem(graph.Context, items, *config.Integration)
		if err != nil {
			return Plan{}, err
		}
		items = append(items, integration)
	}
	ordered, err := topological(items)
	if err != nil {
		return Plan{}, err
	}

	cards := make([]Card, 0, len(ordered))
	workspaceOwners := make(map[string]string, len(ordered))
	for _, item := range ordered {
		workspacePath, err := preparedWorkspace(config.PreparedWorkspaces, item.ID)
		if err != nil {
			return Plan{}, err
		}
		if owner, ok := workspaceOwners[workspacePath]; ok {
			return Plan{}, fmt.Errorf("prepared workspace %q is shared by work items %q and %q", workspacePath, owner, item.ID)
		}
		workspaceOwners[workspacePath] = item.ID
		budget := item.RuntimeBudgetSeconds
		if budget == 0 {
			budget = config.DefaultRuntimeBudgetSeconds
		}
		parents := append([]string(nil), item.Parents...)
		sort.Strings(parents)
		parentRefs := make([]ParentReference, len(parents))
		for i, parent := range parents {
			parentRefs[i] = ParentReference{WorkItemID: parent}
		}
		synthetic := config.Integration != nil && item.ID == config.Integration.ID
		goalMode := ""
		if synthetic {
			goalMode = config.Integration.GoalMode
		}
		commands := cloneCommands(config.LifecycleCommands)
		contextCommand, err := lifecycleContextCommand(workspacePath, graph.Context)
		if err != nil {
			return Plan{}, err
		}
		commands.Prepare = append([]string{contextCommand}, commands.Prepare...)
		commands.Execute = append([]string{contextCommand}, commands.Execute...)
		commands.Validate = append([]string{contextCommand}, commands.Validate...)
		commands.Validate = append(commands.Validate, item.ValidationCommands...)
		requirements, criteria, surfaces, constraints, baseModel := visibleContract(config.Design, item)
		card := Card{
			Identity: WorkItemIdentity{
				Context: graph.Context, Round: config.Round, WorkItemID: item.ID, Synthetic: synthetic,
			},
			Title:                item.Title,
			Body:                 cardBody(graph.Context, workspacePath, item.Prompt),
			Parents:              parentRefs,
			Tenant:               domain.KanbanTenant(graph.Context.ProjectID),
			Priority:             config.Priority,
			WorkspaceKind:        WorkspaceKindWorktree,
			WorkspacePath:        workspacePath,
			AssignedProfile:      item.Worker.Profile,
			ForcedSkills:         cloneSkills(item.Worker.Skills),
			RuntimeBudgetSeconds: budget,
			GoalMode:             goalMode,
			LifecycleCommands:    commands,
			AcceptanceCriteria:   append([]string(nil), item.AcceptanceCriteria...),
			Requirements:         requirements,
			DesignCriteria:       criteria,
			Surfaces:             surfaces,
			Constraints:          constraints,
			BaseModel:            baseModel,
		}
		key, err := idempotencyKey(card)
		if err != nil {
			return Plan{}, err
		}
		card.IdempotencyKey = key
		cards = append(cards, card)
	}
	return Plan{Context: graph.Context, Round: config.Round, Cards: cards}, nil
}

// Ready returns unbound cards whose logical parents are all bound. Parent task
// IDs are resolved in the returned copies; the plan remains reusable.
func (p Plan) Ready(bindings []domain.TaskBinding) ([]Card, error) {
	if err := validateBindingContexts(bindings, p.Context); err != nil {
		return nil, err
	}
	if err := validateBindingRound(bindings, p.Round); err != nil {
		return nil, err
	}
	index, err := bindingIndex(bindings)
	if err != nil {
		return nil, err
	}
	ready := make([]Card, 0, len(p.Cards))
	for _, card := range p.Cards {
		if _, alreadyCreated := index[card.Identity.WorkItemID]; alreadyCreated {
			continue
		}
		resolved, err := ResolveParents(card, bindings)
		if errors.Is(err, ErrMissingParentBinding) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ready = append(ready, resolved)
	}
	return ready, nil
}

func (p Plan) CreateReadyCards(bindings []domain.TaskBinding) ([]Card, error) {
	return p.Ready(bindings)
}

func ResolveParents(card Card, bindings []domain.TaskBinding) (Card, error) {
	if err := validateBindingContexts(bindings, card.Identity.Context); err != nil {
		return Card{}, err
	}
	if err := validateBindingRound(bindings, card.Identity.Round); err != nil {
		return Card{}, err
	}
	index, err := bindingIndex(bindings)
	if err != nil {
		return Card{}, err
	}
	resolved := card
	resolved.Parents = append([]ParentReference(nil), card.Parents...)
	for i := range resolved.Parents {
		taskID, ok := index[resolved.Parents[i].WorkItemID]
		if !ok {
			return Card{}, fmt.Errorf("%w: work item %q requires parent %q",
				ErrMissingParentBinding, card.Identity.WorkItemID, resolved.Parents[i].WorkItemID)
		}
		resolved.Parents[i].TaskID = taskID
	}
	return resolved, nil
}

func validateConfig(config Config) error {
	if config.Priority < 0 {
		return errors.New("dispatch priority must not be negative")
	}
	if config.DefaultRuntimeBudgetSeconds == 0 {
		return errors.New("default runtime budget must be greater than zero")
	}
	for kind, commands := range map[string][]string{
		"prepare":  config.LifecycleCommands.Prepare,
		"execute":  config.LifecycleCommands.Execute,
		"validate": config.LifecycleCommands.Validate,
	} {
		for i, command := range commands {
			if strings.TrimSpace(command) == "" {
				return fmt.Errorf("%s lifecycle command %d must not be empty", kind, i)
			}
		}
	}
	return nil
}

func preparedWorkspace(workspaces map[string]string, workItemID string) (string, error) {
	path, ok := workspaces[workItemID]
	if !ok || path == "" {
		return "", fmt.Errorf("prepared workspace for work item %q is missing", workItemID)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("prepared workspace for work item %q must be an absolute path", workItemID)
	}
	if filepath.Clean(path) != path {
		return "", fmt.Errorf("prepared workspace for work item %q must be a clean path", workItemID)
	}
	return path, nil
}

func integrationItem(context domain.ContextRef, items []domain.WorkItem, integration SyntheticIntegration) (domain.WorkItem, error) {
	if strings.TrimSpace(integration.GoalMode) == "" {
		return domain.WorkItem{}, errors.New("synthetic integration goal mode must not be empty")
	}
	for _, item := range items {
		if item.ID == integration.ID {
			return domain.WorkItem{}, fmt.Errorf("synthetic integration ID %q conflicts with a work item", integration.ID)
		}
	}
	children := make(map[string]bool, len(items))
	for _, item := range items {
		for _, parent := range item.Parents {
			children[parent] = true
		}
	}
	parents := make([]string, 0)
	for _, item := range items {
		if !children[item.ID] {
			parents = append(parents, item.ID)
		}
	}
	sort.Strings(parents)
	item := domain.WorkItem{
		ID:                   integration.ID,
		Title:                integration.Title,
		Prompt:               integration.Body,
		AcceptanceCriteria:   append([]string(nil), integration.AcceptanceCriteria...),
		RequirementIDs:       collectItemReferences(items, func(item domain.WorkItem) []string { return item.RequirementIDs }),
		CriterionIDs:         collectItemReferences(items, func(item domain.WorkItem) []string { return item.CriterionIDs }),
		SurfaceIDs:           collectItemReferences(items, func(item domain.WorkItem) []string { return item.SurfaceIDs }),
		Parents:              parents,
		Worker:               integration.Worker,
		RuntimeBudgetSeconds: integration.RuntimeBudgetSeconds,
	}

	check := domain.WorkGraph{
		SchemaVersion: domain.SchemaVersion,
		Context:       context,
		Producer:      domain.Producer{Skill: "dispatch", Runtime: "deterministic"},
		Revision:      1,
		Items:         append(append([]domain.WorkItem(nil), items...), item),
	}
	if err := check.Validate(); err != nil {
		return domain.WorkItem{}, fmt.Errorf("invalid synthetic integration item: %w", err)
	}

	return item, nil
}

func visibleContract(
	design *domain.FeatureDesign,
	item domain.WorkItem,
) (
	[]domain.Requirement,
	[]domain.AcceptanceCriterion,
	[]domain.FeatureSurface,
	[]string,
	*domain.ModelReference,
) {
	if design == nil {
		return nil, nil, nil, nil, nil
	}
	requirementIDs := sliceSet(item.RequirementIDs)
	criterionIDs := sliceSet(item.CriterionIDs)
	surfaceIDs := sliceSet(item.SurfaceIDs)
	requirements := make([]domain.Requirement, 0, len(requirementIDs))
	for _, requirement := range design.Requirements {
		if _, ok := requirementIDs[requirement.ID]; ok {
			requirements = append(requirements, requirement)
		}
	}
	criteria := make([]domain.AcceptanceCriterion, 0, len(criterionIDs))
	for _, criterion := range design.AcceptanceCriteria {
		if _, ok := criterionIDs[criterion.ID]; ok {
			criteria = append(criteria, criterion)
		}
	}
	surfaces := make([]domain.FeatureSurface, 0, len(surfaceIDs))
	for _, surface := range design.Surfaces {
		if _, ok := surfaceIDs[surface.ID]; ok {
			surfaces = append(surfaces, surface)
		}
	}
	baseModel := design.BaseModel
	return requirements, criteria, surfaces, append([]string(nil), design.Constraints...), &baseModel
}

func collectItemReferences(
	items []domain.WorkItem,
	references func(domain.WorkItem) []string,
) []string {
	set := map[string]struct{}{}
	for _, item := range items {
		for _, id := range references(item) {
			set[id] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func sliceSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func topological(items []domain.WorkItem) ([]domain.WorkItem, error) {
	byID := make(map[string]domain.WorkItem, len(items))
	degree := make(map[string]int, len(items))
	children := make(map[string][]string, len(items))
	for _, item := range items {
		byID[item.ID] = item
		degree[item.ID] = len(item.Parents)
		for _, parent := range item.Parents {
			children[parent] = append(children[parent], item.ID)
		}
	}
	ready := make([]string, 0)
	for id, count := range degree {
		if count == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	ordered := make([]domain.WorkItem, 0, len(items))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		ordered = append(ordered, byID[id])
		sort.Strings(children[id])
		for _, child := range children[id] {
			degree[child]--
			if degree[child] == 0 {
				ready = append(ready, child)
				sort.Strings(ready)
			}
		}
	}
	if len(ordered) != len(items) {
		return nil, errors.New("work graph contains a dependency cycle")
	}
	return ordered, nil
}

func bindingIndex(bindings []domain.TaskBinding) (map[string]string, error) {
	index := make(map[string]string, len(bindings))
	taskOwners := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return nil, fmt.Errorf("invalid task binding: %w", err)
		}

		if existing, ok := index[binding.WorkItemID]; ok && existing != binding.TaskID {
			return nil, fmt.Errorf("conflicting task bindings for work item %q", binding.WorkItemID)
		}
		if owner, ok := taskOwners[binding.TaskID]; ok && owner != binding.WorkItemID {
			return nil, fmt.Errorf("task %q is bound to both %q and %q", binding.TaskID, owner, binding.WorkItemID)
		}
		index[binding.WorkItemID] = binding.TaskID
		taskOwners[binding.TaskID] = binding.WorkItemID
	}
	return index, nil
}

func validateBindingContexts(bindings []domain.TaskBinding, expected domain.ContextRef) error {
	for _, binding := range bindings {
		if !binding.Context.Equal(expected) {
			return errors.New("task binding context does not match dispatch context")
		}
	}
	return nil
}

func validateBindingRound(bindings []domain.TaskBinding, expected uint64) error {
	for _, binding := range bindings {
		if binding.Round != expected {
			return errors.New("task binding round does not match dispatch round")
		}
	}
	return nil
}

func cardBody(context domain.ContextRef, workspace, prompt string) string {
	data, _ := json.Marshal(context)
	repository, _ := posixQuote(context.Repository.Repository)
	return fmt.Sprintf(
		"Execution context (refresh with `hermoso context %s %s %s %s --json` and block on any mismatch):\n%s\nAbsolute workspace: %s\n\n%s",
		context.ProjectID, context.FeatureID, context.RunID, repository, data, workspace, prompt,
	)
}

func lifecycleContextCommand(workspace string, context domain.ContextRef) (string, error) {
	values := []string{
		workspace,
		context.ProjectID,
		context.FeatureID,
		context.RunID,
		context.Repository.Repository,
	}
	quoted := make([]string, len(values))
	for i, value := range values {
		var err error
		quoted[i], err = posixQuote(value)
		if err != nil {
			return "", fmt.Errorf("lifecycle context argument: %w", err)
		}
	}
	return fmt.Sprintf(
		"test \"$PWD\" = %s && hermoso context %s %s %s %s --json",
		quoted[0], quoted[1], quoted[2], quoted[3], quoted[4],
	), nil
}

func posixQuote(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n") {
		return "", errors.New("newlines are not allowed")
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'", nil
}

func idempotencyKey(card Card) (string, error) {
	card.IdempotencyKey = ""
	data, err := json.Marshal(card)
	if err != nil {
		return "", fmt.Errorf("encode card idempotency input: %w", err)
	}
	sum := sha256.Sum256(data)
	return "hermoso-card-sha256:" + hex.EncodeToString(sum[:]), nil
}

func cloneSkills(skills []domain.SkillBinding) []domain.SkillBinding {
	return append([]domain.SkillBinding(nil), skills...)
}

func cloneCommands(commands LifecycleCommands) LifecycleCommands {
	return LifecycleCommands{
		Prepare:  append([]string(nil), commands.Prepare...),
		Execute:  append([]string(nil), commands.Execute...),
		Validate: append([]string(nil), commands.Validate...),
	}
}
