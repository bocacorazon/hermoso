package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrInvalidName = errors.New("invalid managed Git name")
	ErrPathEscape  = errors.New("managed worktree path escapes its root")
	ErrDirty       = errors.New("managed worktree is dirty")
	ErrConflict    = errors.New("merge is blocked by conflicts")
	ErrUnsafeReuse = errors.New("existing branch or path cannot be safely reused")
	ErrNoMerge     = errors.New("no managed merge is in progress")
)

type Manager struct {
	root      string
	worktrees string
	namespace string
}

type Worktree struct {
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	Branch       string `json:"branch"`
	Path         string `json:"path"`
	ParentBranch string `json:"parent_branch"`
	ParentCommit string `json:"parent_commit"`
}

type MergeState struct {
	Worktree       Worktree          `json:"worktree"`
	SourceBranch   string            `json:"source_branch"`
	SourceCommit   string            `json:"source_commit"`
	TargetCommit   string            `json:"target_commit"`
	Conflicted     bool              `json:"conflicted"`
	ConflictPaths  []string          `json:"conflict_paths,omitempty"`
	ConflictHashes map[string]string `json:"conflict_hashes,omitempty"`
}

type CheckResult struct {
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
}

type IntegrationNode struct {
	ID       string `json:"id"`
	Left     string `json:"left"`
	Right    string `json:"right,omitempty"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
}

type IntegrationResult struct {
	Nodes  []IntegrationNode `json:"nodes"`
	Merge  *MergeState       `json:"merge,omitempty"`
	Block  *BlockState       `json:"block,omitempty"`
	Checks []CheckResult     `json:"checks,omitempty"`
}

type BlockState struct {
	Kind     string       `json:"kind"`
	Worktree Worktree     `json:"worktree"`
	Merge    *MergeState  `json:"merge,omitempty"`
	Check    *CheckResult `json:"check,omitempty"`
	Message  string       `json:"message"`
}

func NewManager(root, worktreeRoot, namespace string) (*Manager, error) {
	repositoryRoot, err := git(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("discover repository: %w", err)
	}
	repositoryRoot, err = filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("canonicalize repository: %w", err)
	}
	if namespace == "" {
		namespace = "hermoso"
	}
	if err := validateComponent(namespace); err != nil {
		return nil, fmt.Errorf("namespace: %w", err)
	}
	if worktreeRoot == "" {
		worktreeRoot = filepath.Join(repositoryRoot, ".hermoso", "worktrees")
	}
	worktreeRoot, err = filepath.Abs(worktreeRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve worktree root: %w", err)
	}
	worktreeRoot = filepath.Clean(worktreeRoot)
	if err := rejectSymlinkPath(worktreeRoot); err != nil {
		return nil, fmt.Errorf("%w: worktree root: %v", ErrPathEscape, err)
	}
	return &Manager{root: repositoryRoot, worktrees: worktreeRoot, namespace: namespace}, nil
}

func (m *Manager) Feature(ctx context.Context, runID, featureID, parentBranch string) (Worktree, error) {
	if err := validateComponent(runID); err != nil {
		return Worktree{}, fmt.Errorf("run ID: %w", err)
	}
	if err := validateComponent(featureID); err != nil {
		return Worktree{}, fmt.Errorf("feature ID: %w", err)
	}
	if err := validateBranch(parentBranch); err != nil {
		return Worktree{}, fmt.Errorf("parent branch: %w", err)
	}
	return m.ensure(ctx, Worktree{
		Kind:         "feature",
		ID:           featureID,
		Branch:       m.namespace + "/run/" + runID + "/feature/" + featureID,
		Path:         filepath.Join(m.worktrees, runID, "feature"),
		ParentBranch: parentBranch,
	})
}

func (m *Manager) WorkItem(ctx context.Context, runID, featureID, itemID string, parents []Worktree) (Worktree, *MergeState, error) {
	for _, value := range []struct {
		name, value string
	}{{"run ID", runID}, {"feature ID", featureID}, {"work item ID", itemID}} {
		if err := validateComponent(value.value); err != nil {
			return Worktree{}, nil, fmt.Errorf("%s: %w", value.name, err)
		}
	}
	if len(parents) == 0 {
		return Worktree{}, nil, fmt.Errorf("work item requires at least one parent worktree")
	}
	parentCommits, err := m.resolveParents(parents)
	if err != nil {
		return Worktree{}, nil, err
	}
	base := parents[0].Branch
	wanted := Worktree{
		Kind:         "work-item",
		ID:           itemID,
		Branch:       m.namespace + "/run/" + runID + "/work/" + featureID + "/" + itemID,
		Path:         filepath.Join(m.worktrees, runID, "items", itemID),
		ParentBranch: base,
		ParentCommit: parentCommits[base],
	}
	if inProgress(wanted.Path) {
		mergeHead, mergeErr := git(wanted.Path, "rev-parse", "MERGE_HEAD")
		if mergeErr != nil {
			return Worktree{}, nil, mergeErr
		}
		for _, parent := range parents {
			if parentCommits[parent.Branch] != mergeHead {
				continue
			}
			wanted.Path, err = m.safePath(wanted.Path)
			if err != nil {
				return Worktree{}, nil, err
			}
			if err := m.validateManaged(wanted); err != nil {
				return Worktree{}, nil, err
			}
			merge, err := m.mergeState(wanted, parent.Branch)
			if err != nil {
				return Worktree{}, nil, err
			}
			if merge.Conflicted {
				return wanted, &merge, ErrConflict
			}
			if _, err := m.ContinueMerge(ctx, merge); err != nil {
				return wanted, &merge, err
			}
			break
		}
	}
	item, err := m.ensure(ctx, wanted)
	if err != nil {
		return Worktree{}, nil, err
	}
	for _, parent := range sortedWorktrees(parents) {
		state, err := m.Merge(ctx, item, parent.Branch)
		if errors.Is(err, ErrConflict) {
			return item, &state, err
		}
		if err != nil {
			return item, nil, err
		}
	}
	return item, nil, nil
}

func (m *Manager) ensure(ctx context.Context, wanted Worktree) (Worktree, error) {
	if err := ctx.Err(); err != nil {
		return Worktree{}, err
	}
	if err := validateBranch(wanted.Branch); err != nil {
		return Worktree{}, err
	}
	path, err := m.safePath(wanted.Path)
	if err != nil {
		return Worktree{}, err
	}
	wanted.Path = path
	parentCommit, err := git(m.root, "rev-parse", "--verify", wanted.ParentBranch+"^{commit}")
	if err != nil {
		return Worktree{}, fmt.Errorf("resolve parent branch %q: %w", wanted.ParentBranch, err)
	}
	if wanted.ParentCommit == "" {
		wanted.ParentCommit = parentCommit
	} else if wanted.ParentCommit != parentCommit {
		return Worktree{}, fmt.Errorf("%w: parent %q moved from %s to %s", ErrUnsafeReuse, wanted.ParentBranch, wanted.ParentCommit, parentCommit)
	}

	registered, err := m.registeredWorktree(path)
	if err != nil {
		return Worktree{}, err
	}
	branchExists := gitSuccess(m.root, "show-ref", "--verify", "--quiet", "refs/heads/"+wanted.Branch)
	if registered != "" {
		if registered != wanted.Branch {
			return Worktree{}, fmt.Errorf("%w: %q is registered for branch %q, want %q", ErrUnsafeReuse, path, registered, wanted.Branch)
		}
		if err := m.verifyOwnership(wanted); err != nil {
			return Worktree{}, err
		}
		if inProgress(wanted.Path) {
			return wanted, nil
		}
		if err := m.requireClean(wanted); err != nil {
			return Worktree{}, err
		}
		return wanted, nil
	}
	if _, err := os.Lstat(path); err == nil {
		return Worktree{}, fmt.Errorf("%w: unmanaged path already exists: %s", ErrUnsafeReuse, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Worktree{}, fmt.Errorf("inspect worktree path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Worktree{}, fmt.Errorf("create worktree parent: %w", err)
	}
	args := []string{"worktree", "add"}
	if branchExists {
		if err := m.verifyOwnership(wanted); err != nil {
			return Worktree{}, err
		}
		args = append(args, path, wanted.Branch)
	} else {
		args = append(args, "-b", wanted.Branch, path, wanted.ParentCommit)
	}
	if _, err := git(m.root, args...); err != nil {
		return Worktree{}, fmt.Errorf("create worktree: %w", err)
	}
	if !branchExists {
		for key, value := range map[string]string{
			"managed":       m.namespace,
			"kind":          wanted.Kind,
			"id":            wanted.ID,
			"origin-parent": wanted.ParentCommit,
		} {
			if _, err := git(m.root, "config", "--local", m.metadataKey(wanted.Branch, key), value); err != nil {
				return Worktree{}, fmt.Errorf("record managed branch ownership: %w", err)
			}
		}
	}
	return wanted, nil
}

func (m *Manager) verifyOwnership(worktree Worktree) error {
	expected := map[string]string{
		"managed": m.namespace,
		"kind":    worktree.Kind,
		"id":      worktree.ID,
	}
	for key, want := range expected {
		got, err := git(m.root, "config", "--local", "--get", m.metadataKey(worktree.Branch, key))
		if err != nil || got != want {
			return fmt.Errorf("%w: branch %q lacks matching managed ownership", ErrUnsafeReuse, worktree.Branch)
		}
	}
	origin, err := git(m.root, "config", "--local", "--get", m.metadataKey(worktree.Branch, "origin-parent"))
	if err != nil || !gitSuccess(m.root, "merge-base", "--is-ancestor", origin, worktree.Branch) {
		return fmt.Errorf("%w: branch %q no longer contains its recorded origin", ErrUnsafeReuse, worktree.Branch)
	}
	return nil
}

func (m *Manager) metadataKey(branch, key string) string {
	return "branch." + branch + ".hermoso-" + key
}

func (m *Manager) Merge(ctx context.Context, target Worktree, sourceBranch string) (MergeState, error) {
	if err := ctx.Err(); err != nil {
		return MergeState{}, err
	}
	if err := validateBranch(sourceBranch); err != nil {
		return MergeState{}, err
	}
	if err := m.validateManaged(target); err != nil {
		return MergeState{}, err
	}
	if inProgress(target.Path) {
		state, err := m.mergeState(target, sourceBranch)
		if err != nil {
			return MergeState{}, err
		}
		return state, ErrConflict
	}
	if err := m.requireClean(target); err != nil {
		return MergeState{}, err
	}
	sourceCommit, err := git(m.root, "rev-parse", "--verify", sourceBranch+"^{commit}")
	if err != nil {
		return MergeState{}, fmt.Errorf("resolve merge source %q: %w", sourceBranch, err)
	}
	targetCommit, err := git(target.Path, "rev-parse", "HEAD")
	if err != nil {
		return MergeState{}, err
	}
	state := MergeState{Worktree: target, SourceBranch: sourceBranch, SourceCommit: sourceCommit, TargetCommit: targetCommit}
	if gitSuccess(target.Path, "merge-base", "--is-ancestor", sourceCommit, targetCommit) {
		return state, nil
	}
	_, err = git(target.Path, "merge", "--no-ff", "--no-edit", sourceCommit)
	if err == nil {
		return state, nil
	}
	if inProgress(target.Path) {
		conflicts, conflictErr := conflictPaths(target.Path)
		if conflictErr != nil {
			return MergeState{}, conflictErr
		}
		state.Conflicted = true
		state.ConflictPaths = conflicts
		state.ConflictHashes, conflictErr = hashPaths(target.Path, conflicts)
		if conflictErr != nil {
			return MergeState{}, conflictErr
		}
		return state, fmt.Errorf("%w: merge %s into %s", ErrConflict, sourceBranch, target.Branch)
	}
	return MergeState{}, fmt.Errorf("merge %s into %s: %w", sourceBranch, target.Branch, err)
}

func (m *Manager) ContinueMerge(ctx context.Context, state MergeState) (MergeState, error) {
	if err := ctx.Err(); err != nil {
		return MergeState{}, err
	}
	if err := m.validateManaged(state.Worktree); err != nil {
		return MergeState{}, err
	}
	if !inProgress(state.Worktree.Path) {
		return MergeState{}, ErrNoMerge
	}
	conflicts, err := conflictPaths(state.Worktree.Path)
	if err != nil {
		return MergeState{}, err
	}
	if len(conflicts) != 0 {
		state.Conflicted = true
		state.ConflictPaths = conflicts
		return state, ErrConflict
	}
	head, err := git(state.Worktree.Path, "rev-parse", "HEAD")
	if err != nil {
		return MergeState{}, err
	}
	if head != state.TargetCommit {
		return MergeState{}, fmt.Errorf("%w: merge target changed from %s to %s", ErrUnsafeReuse, state.TargetCommit, head)
	}
	source, err := git(m.root, "rev-parse", "--verify", state.SourceBranch+"^{commit}")
	if err != nil {
		return MergeState{}, err
	}
	if source != state.SourceCommit {
		return MergeState{}, fmt.Errorf("%w: merge source moved from %s to %s", ErrUnsafeReuse, state.SourceCommit, source)
	}
	if _, err := git(state.Worktree.Path, "commit", "--no-edit"); err != nil {
		return MergeState{}, fmt.Errorf("continue merge: %w", err)
	}
	state.Conflicted = false
	state.ConflictPaths = nil
	state.ConflictHashes = nil
	return state, nil
}

func (m *Manager) AbortMerge(ctx context.Context, state MergeState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.validateManaged(state.Worktree); err != nil {
		return err
	}
	if !inProgress(state.Worktree.Path) {
		return ErrNoMerge
	}
	head, err := git(state.Worktree.Path, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != state.TargetCommit {
		return fmt.Errorf("%w: refusing to abort after target HEAD changed", ErrUnsafeReuse)
	}
	currentConflicts, err := conflictPaths(state.Worktree.Path)
	if err != nil {
		return err
	}
	if strings.Join(currentConflicts, "\x00") != strings.Join(state.ConflictPaths, "\x00") {
		return fmt.Errorf("%w: conflict resolution has changed; refusing to discard it", ErrDirty)
	}
	currentHashes, err := hashPaths(state.Worktree.Path, currentConflicts)
	if err != nil {
		return err
	}
	for path, original := range state.ConflictHashes {
		if currentHashes[path] != original {
			return fmt.Errorf("%w: conflict file %q was edited; refusing to discard it", ErrDirty, path)
		}
	}
	if _, err := git(state.Worktree.Path, "merge", "--abort"); err != nil {
		return fmt.Errorf("abort merge: %w", err)
	}
	return nil
}

func (m *Manager) Integrate(ctx context.Context, runID, featureID string, feature Worktree, leaves []Worktree, checks []string) (IntegrationResult, error) {
	if err := m.validateManaged(feature); err != nil {
		return IntegrationResult{}, err
	}
	if !inProgress(feature.Path) {
		if err := m.requireClean(feature); err != nil {
			return IntegrationResult{}, err
		}
	}
	if len(leaves) == 0 {
		return IntegrationResult{}, errors.New("integration requires at least one graph leaf")
	}
	allIntegrated := true
	featureCommit, err := git(feature.Path, "rev-parse", "HEAD")
	if err != nil {
		return IntegrationResult{}, err
	}
	for _, leaf := range leaves {
		if err := m.validateManaged(leaf); err != nil {
			return IntegrationResult{}, err
		}
		if err := m.requireClean(leaf); err != nil {
			return IntegrationResult{}, err
		}
		leafCommit, err := git(leaf.Path, "rev-parse", "HEAD")
		if err != nil {
			return IntegrationResult{}, err
		}
		if !gitSuccess(m.root, "merge-base", "--is-ancestor", leafCommit, featureCommit) {
			allIntegrated = false
		}
	}
	if allIntegrated {
		checkResults, err := runChecks(ctx, feature.Path, checks)
		result := IntegrationResult{Checks: checkResults}
		if err != nil {
			result.Block = checkBlock(feature, checkResults, err)
		}
		return result, err
	}
	current := sortedWorktrees(leaves)
	result := IntegrationResult{}
	nodeNumber := 0
	for len(current) > 1 {
		next := make([]Worktree, 0, (len(current)+1)/2)
		for i := 0; i < len(current); i += 2 {
			if i+1 == len(current) {
				next = append(next, current[i])
				continue
			}
			nodeNumber++
			id := "integration-" + strconv.Itoa(nodeNumber)
			nodeBranch := m.namespace + "/run/" + runID + "/integration/" + featureID + "/" + id
			node, err := m.ensure(ctx, Worktree{
				Kind:         "integration",
				ID:           id,
				Branch:       nodeBranch,
				Path:         filepath.Join(m.worktrees, runID, "integration", id),
				ParentBranch: feature.Branch,
			})
			if err != nil {
				return result, err
			}
			merge, err := m.mergeSources(ctx, node, current[i:i+2])
			if errors.Is(err, ErrConflict) {
				result.Merge = &merge
				result.Block = &BlockState{
					Kind: "merge_conflict", Worktree: node, Merge: &merge,
					Message: err.Error(),
				}
				return result, err
			}
			if err != nil {
				return result, err
			}
			result.Nodes = append(result.Nodes, IntegrationNode{
				ID: id, Left: current[i].Branch, Right: current[i+1].Branch,
				Branch: node.Branch, Worktree: node.Path,
			})
			next = append(next, node)
		}
		current = next
	}
	merge, err := m.mergeOrContinue(ctx, feature, current[0].Branch)
	if errors.Is(err, ErrConflict) {
		result.Merge = &merge
		result.Block = &BlockState{
			Kind: "merge_conflict", Worktree: feature, Merge: &merge,
			Message: err.Error(),
		}
		return result, err
	}
	if err != nil {
		return result, err
	}

	result.Checks, err = runChecks(ctx, feature.Path, checks)
	if err != nil {
		result.Block = checkBlock(feature, result.Checks, err)
	}
	return result, err
}

func (m *Manager) mergeOrContinue(ctx context.Context, target Worktree, sourceBranch string) (MergeState, error) {
	merge, err := m.Merge(ctx, target, sourceBranch)
	if !errors.Is(err, ErrConflict) || merge.Conflicted {
		return merge, err
	}
	return m.ContinueMerge(ctx, merge)
}

func (m *Manager) mergeSources(ctx context.Context, target Worktree, sources []Worktree) (MergeState, error) {
	if inProgress(target.Path) {
		mergeHead, err := git(target.Path, "rev-parse", "MERGE_HEAD")
		if err != nil {
			return MergeState{}, err
		}
		matched := false
		for _, source := range sources {
			sourceCommit, err := git(m.root, "rev-parse", "--verify", source.Branch+"^{commit}")
			if err != nil {
				return MergeState{}, err
			}
			if sourceCommit != mergeHead {
				continue
			}
			matched = true
			state, err := m.mergeState(target, source.Branch)
			if err != nil {
				return MergeState{}, err
			}
			if state.Conflicted {
				return state, ErrConflict
			}
			if _, err := m.ContinueMerge(ctx, state); err != nil {
				return state, err
			}
			break
		}
		if !matched {
			return MergeState{}, fmt.Errorf("%w: active merge source %s is not an expected integration source", ErrUnsafeReuse, mergeHead)
		}
	}
	var last MergeState
	for _, source := range sources {
		var err error
		last, err = m.mergeOrContinue(ctx, target, source.Branch)
		if err != nil {
			return last, err
		}
	}
	return last, nil
}

func (m *Manager) CurrentCommit(worktree Worktree) (string, error) {
	if err := m.validateManaged(worktree); err != nil {
		return "", err
	}
	if inProgress(worktree.Path) {
		return "", fmt.Errorf("%w: merge is in progress in %s", ErrUnsafeReuse, worktree.Path)
	}
	if err := m.requireClean(worktree); err != nil {
		return "", err
	}
	head, err := git(worktree.Path, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	branch, err := git(m.root, "rev-parse", "--verify", worktree.Branch+"^{commit}")
	if err != nil {
		return "", err
	}
	if head != branch {
		return "", fmt.Errorf("%w: worktree %s HEAD does not match branch %s", ErrUnsafeReuse, worktree.Path, worktree.Branch)
	}
	return head, nil
}

func checkBlock(worktree Worktree, checks []CheckResult, err error) *BlockState {
	block := &BlockState{Kind: "baseline_check", Worktree: worktree, Message: err.Error()}
	if len(checks) != 0 {
		block.Check = &checks[len(checks)-1]
	}
	return block
}

func runChecks(ctx context.Context, path string, checks []string) ([]CheckResult, error) {
	results := make([]CheckResult, 0, len(checks))
	for _, command := range checks {
		command = strings.TrimSpace(command)
		if command == "" {
			return results, errors.New("baseline check must not be empty")
		}
		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = path
		output, err := cmd.CombinedOutput()
		check := CheckResult{Command: command, Output: strings.TrimSpace(string(output))}
		results = append(results, check)
		if err != nil {
			return results, fmt.Errorf("baseline check %q failed: %w: %s", command, err, check.Output)
		}
	}
	return results, nil
}

func (m *Manager) resolveParents(parents []Worktree) (map[string]string, error) {
	commits := make(map[string]string, len(parents))
	for _, parent := range parents {
		if err := m.validateManaged(parent); err != nil {
			return nil, err
		}
		if err := m.requireClean(parent); err != nil {
			return nil, err
		}
		commit, err := git(parent.Path, "rev-parse", "HEAD")
		if err != nil {
			return nil, err
		}
		refCommit, err := git(m.root, "rev-parse", "--verify", parent.Branch+"^{commit}")
		if err != nil {
			return nil, err
		}
		if commit != refCommit {
			return nil, fmt.Errorf("%w: worktree %s HEAD does not match branch %s", ErrUnsafeReuse, parent.Path, parent.Branch)
		}
		commits[parent.Branch] = commit
	}
	return commits, nil
}

func (m *Manager) validateManaged(worktree Worktree) error {
	path, err := m.safePath(worktree.Path)
	if err != nil {
		return err
	}
	if path != filepath.Clean(worktree.Path) {
		return fmt.Errorf("%w: non-canonical worktree path", ErrPathEscape)
	}
	branch, err := m.registeredWorktree(path)
	if err != nil {
		return err
	}
	if branch == "" || branch != worktree.Branch {
		return fmt.Errorf("%w: %q is not registered for branch %q", ErrUnsafeReuse, path, worktree.Branch)
	}
	return nil
}

func (m *Manager) requireClean(worktree Worktree) error {
	status, err := git(worktree.Path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("%w: %s", ErrDirty, worktree.Path)
	}
	return nil
}

func (m *Manager) safePath(path string) (string, error) {
	if err := rejectSymlinkPath(m.worktrees); err != nil {
		return "", fmt.Errorf("%w: worktree root: %v", ErrPathEscape, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	relative, err := filepath.Rel(m.worktrees, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: %s", ErrPathEscape, path)
	}
	for probe := absolute; ; probe = filepath.Dir(probe) {
		info, err := os.Lstat(probe)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: symlink is not allowed: %s", ErrPathEscape, probe)
		}
		if probe == m.worktrees || probe == filepath.Dir(probe) {
			break
		}
	}
	return absolute, nil
}

func rejectSymlinkPath(path string) error {
	path = filepath.Clean(path)
	for probe := path; ; probe = filepath.Dir(probe) {
		info, err := os.Lstat(probe)
		if errors.Is(err, os.ErrNotExist) {
			if probe == filepath.Dir(probe) {
				return nil
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", probe)
		}
		if probe == filepath.Dir(probe) {
			return nil
		}
	}
}

func (m *Manager) registeredWorktree(path string) (string, error) {
	output, err := git(m.root, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	var candidate, branch string
	for _, line := range strings.Split(output+"\n", "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			candidate = filepath.Clean(strings.TrimPrefix(line, "worktree "))
			branch = ""
		case strings.HasPrefix(line, "branch "):
			branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "":
			if candidate == path {
				return branch, nil
			}
			candidate, branch = "", ""
		}
	}
	return "", nil
}

func (m *Manager) mergeState(target Worktree, source string) (MergeState, error) {
	sourceCommit, err := git(m.root, "rev-parse", "--verify", source+"^{commit}")
	if err != nil {
		return MergeState{}, err
	}
	mergeCommit, err := git(target.Path, "rev-parse", "MERGE_HEAD")
	if err != nil {
		return MergeState{}, err
	}
	if sourceCommit != mergeCommit {
		return MergeState{}, fmt.Errorf("%w: merge in progress uses %s, not %s", ErrUnsafeReuse, mergeCommit, sourceCommit)
	}
	targetCommit, err := git(target.Path, "rev-parse", "HEAD")
	if err != nil {
		return MergeState{}, err
	}
	conflicts, err := conflictPaths(target.Path)
	if err != nil {
		return MergeState{}, err
	}
	hashes, err := hashPaths(target.Path, conflicts)
	if err != nil {
		return MergeState{}, err
	}
	return MergeState{
		Worktree: target, SourceBranch: source, SourceCommit: sourceCommit,
		TargetCommit: targetCommit, Conflicted: len(conflicts) != 0, ConflictPaths: conflicts,
		ConflictHashes: hashes,
	}, nil
}

func validateComponent(value string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return ErrInvalidName
	}
	if !gitSuccess("", "check-ref-format", "--branch", "x-"+value) {
		return ErrInvalidName
	}
	return nil
}

func validateBranch(value string) error {
	if value == "" || strings.HasPrefix(value, "-") || !gitSuccess("", "check-ref-format", "--branch", value) {
		return ErrInvalidName
	}
	return nil
}

func sortedWorktrees(items []Worktree) []Worktree {
	result := append([]Worktree(nil), items...)
	sort.Slice(result, func(i, j int) bool { return result[i].Branch < result[j].Branch })
	return result
}

func conflictPaths(path string) ([]string, error) {
	output, err := git(path, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil, err
	}
	if output == "" {
		return nil, nil
	}
	paths := strings.Split(strings.TrimSuffix(output, "\x00"), "\x00")
	sort.Strings(paths)
	return paths, nil
}

func hashPaths(root string, paths []string) (map[string]string, error) {
	hashes := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if errors.Is(err, os.ErrNotExist) {
			hashes[path] = "missing"
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("hash conflict path %q: %w", path, err)
		}
		hashes[path] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return hashes, nil
}

func inProgress(path string) bool {
	gitDir, err := git(path, "rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(path, gitDir)
	}
	_, err = os.Stat(filepath.Join(gitDir, "MERGE_HEAD"))
	return err == nil
}

func gitSuccess(dir string, args ...string) bool {
	_, err := git(dir, args...)
	return err == nil
}

func git(dir string, args ...string) (string, error) {
	command := append([]string(nil), args...)
	if dir != "" {
		command = append([]string{"-C", dir}, command...)
	}
	cmd := exec.Command("git", command...)
	output, err := cmd.CombinedOutput()
	value := strings.TrimSpace(string(output))
	if err != nil {
		if value != "" {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), value)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return value, nil
}
