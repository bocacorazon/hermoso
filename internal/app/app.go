// Package app contains Hermoso's command parsing and process-level orchestration.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/bocacorazon/hermoso/internal/contracts"
	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/repository"
	"github.com/bocacorazon/hermoso/internal/state"
	"github.com/bocacorazon/hermoso/internal/workflow"
)

const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ErrorUsage      = "usage"
	ErrorInternal   = "internal"
	ErrorValidation = "validation"
	ErrorState      = "state"
)

const usage = `Hermoso is the deterministic control plane for a TUI-first development workflow.

Usage:
  hermoso [--json] <command> [arguments]
  hermoso help
  hermoso version
  hermoso schema <feature-design|work-graph|phase-result>
  hermoso validate <feature-design|work-graph|phase-result> <path> <project-id> <feature-id> <run-id> <repository>
  hermoso init <repository>
  hermoso start <feature-id> <repository>
  hermoso status <repository>
  hermoso context <project-id> <feature-id> <run-id> <repository>
  hermoso design put <project-id> <feature-id> <run-id> <repository> <path>
  hermoso approve design <project-id> <feature-id> <run-id> <repository> <revision> <hash> <actor> [comment]
  hermoso graph put <project-id> <feature-id> <run-id> <repository> <path>
  hermoso construction <prepare|ready|integrate> ...
  hermoso task bind <project-id> <feature-id> <run-id> <repository> <work-item-id> <task-id>
  hermoso work <start|complete|block> ...
  hermoso result put <project-id> <feature-id> <run-id> <repository> <path>
  hermoso resume <project-id> <feature-id> <run-id> <repository>

Commands:
  help       Show this help
  version    Print the Hermoso version
  schema     Print a live versioned contract schema
  validate   Validate a JSON contract file
  init       Initialize clone-local Hermoso state
  start      Create a design-phase run
  status     Show project and run state
  context    Resolve and validate one canonical execution context
  design     Persist a context-bound feature design
  approve    Approve the exact current design revision and hash
  graph      Persist a context-bound construction work graph
  construction Prepare workspaces, emit ready cards, or integrate branches
  task       Record an external Hermes Kanban task binding
  work       Start, complete, or block a synchronized work item
  result     Persist a construction phase result
  resume     Resume blocked construction without resetting user work

Options:
  -h, --help  Show this help
  --json      Emit a stable JSON response
  --version   Print the Hermoso version
`

type Dependencies struct {
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	FS      Filesystem
	Version string
	Now     func() time.Time
}

type application struct {
	deps Dependencies
	out  output
}

func Run(ctx context.Context, args []string, deps Dependencies) int {
	if err := validateDependencies(deps); err != nil {
		fmt.Fprintln(deps.Stderr, err)
		return ExitFailure
	}

	jsonOutput, args := takeJSONFlag(args)
	a := application{
		deps: deps,
		out: output{
			json:   jsonOutput,
			stdout: deps.Stdout,
			stderr: deps.Stderr,
		},
	}
	return a.run(ctx, args)
}

func (a application) run(ctx context.Context, args []string) int {
	if err := ctx.Err(); err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}

	if len(args) == 0 {
		return a.out.success("help", map[string]any{"usage": usage}, usage)
	}

	switch args[0] {
	case "help", "-h", "--help":
		if len(args) != 1 {
			return a.out.usageError("help does not accept arguments")
		}
		return a.out.success("help", map[string]any{"usage": usage}, usage)
	case "version", "--version":
		if len(args) != 1 {
			return a.out.usageError("version does not accept arguments")
		}
		return a.out.success(
			"version",
			map[string]any{"version": a.deps.Version},
			a.deps.Version+"\n",
		)
	case "schema":
		return a.runSchema(args[1:])
	case "validate":
		return a.runValidate(ctx, args[1:])
	case "init":
		return a.runInit(ctx, args[1:])
	case "start":
		return a.runStart(ctx, args[1:])
	case "status":
		return a.runStatus(ctx, args[1:])
	case "context":
		return a.runContext(ctx, args[1:])
	case "design":
		return a.runDesign(ctx, args[1:])
	case "approve":
		return a.runApprove(ctx, args[1:])
	case "graph":
		return a.runGraph(ctx, args[1:])
	case "construction":
		return a.runConstruction(ctx, args[1:])
	case "task":
		return a.runTask(ctx, args[1:])
	case "work":
		return a.runWork(ctx, args[1:])
	case "result":
		return a.runResult(ctx, args[1:])
	case "resume":
		return a.runResume(ctx, args[1:])
	default:
		return a.out.usageError(fmt.Sprintf("unknown command %q", args[0]))
	}
}

func (a application) runInit(ctx context.Context, args []string) int {
	if len(args) != 1 {
		return a.out.usageError("init requires exactly one repository path")
	}
	path := args[0]
	project, created, err := state.Initialize(ctx, path, a.deps.Now())
	if err != nil {
		return a.stateFailure(err)
	}
	action := "already initialized"
	if created {
		action = "initialized"
	}
	return a.out.success("init", map[string]any{
		"created": created,
		"project": project,
	}, fmt.Sprintf("%s Hermoso project %s in %s\n", action, project.ProjectID, project.Target.Repository))
}

func (a application) runStart(ctx context.Context, args []string) int {
	if len(args) != 2 {
		return a.out.usageError("start requires a feature ID and repository path")
	}
	path := args[1]
	store, _, err := state.Load(ctx, path)
	if err != nil {
		return a.stateFailure(err)
	}
	run, err := store.StartRun(ctx, args[0], a.deps.Now())
	if err != nil {
		return a.stateFailure(err)
	}
	return a.out.success("start", map[string]any{
		"run": run,
	}, fmt.Sprintf("started run %s for feature %s\n", run.Context.RunID, run.Context.FeatureID))
}

func (a application) runStatus(ctx context.Context, args []string) int {
	if len(args) != 1 {
		return a.out.usageError("status requires exactly one repository path")
	}
	path := args[0]
	_, status, err := state.Load(ctx, path)
	if err != nil {
		return a.stateFailure(err)
	}

	return a.out.success("status", map[string]any{
		"project": status.Project,
		"runs":    status.Runs,
	}, fmt.Sprintf("project %s: %d run(s)\n", status.Project.ProjectID, len(status.Runs)))
}

func (a application) runContext(ctx context.Context, args []string) int {
	store, execution, _, err := a.resolve(ctx, args)
	if err != nil {
		return a.stateFailure(err)
	}
	_ = store
	return a.out.success("context", map[string]any{
		"context": execution,
	}, fmt.Sprintf("%s %s %s %s\n", execution.ProjectID, execution.FeatureID, execution.RunID, execution.Repository.Repository))
}

func (a application) resolve(ctx context.Context, args []string) (state.Store, domain.ContextRef, []string, error) {
	if len(args) < 4 {
		return state.Store{}, domain.ContextRef{}, nil, fmt.Errorf("%w: full context requires project ID, feature ID, run ID, and repository", state.ErrIncompatibleState)
	}
	store, _, err := state.Load(ctx, args[3])
	if err != nil {
		return state.Store{}, domain.ContextRef{}, nil, err
	}
	execution, err := store.Context(ctx, args[2])
	if err != nil {
		return state.Store{}, domain.ContextRef{}, nil, err
	}
	if execution.ProjectID != args[0] || execution.FeatureID != args[1] {
		return state.Store{}, domain.ContextRef{}, nil, fmt.Errorf("%w: supplied project or feature does not match persisted run", state.ErrIncompatibleState)
	}
	return store, execution, args[4:], nil
}

func (a application) service(store state.Store) (workflow.Service, error) {
	manager, err := repository.NewManager(store.Repository().Root, filepath.Join(store.Repository().Root, ".hermoso", "worktrees"), "hermoso")
	if err != nil {
		return workflow.Service{}, err
	}
	return workflow.Service{Store: store, Manager: manager, Now: a.deps.Now}, nil
}

func (a application) runDesign(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] != "put" {
		return a.out.usageError("design requires: put <project-id> <feature-id> <run-id> <repository> <path>")
	}
	store, execution, rest, err := a.resolve(ctx, args[1:])
	if err != nil {
		return a.stateFailure(err)
	}
	if len(rest) != 1 {
		return a.out.usageError("design put requires exactly one contract path after full context")
	}
	data, err := a.deps.FS.ReadFile(rest[0])
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	service, err := a.service(store)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	run, changed, err := service.PutDesign(ctx, execution, data)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorState, err.Error())
	}
	return a.out.success("design put", map[string]any{"run": run, "design": run.Design, "changed": changed}, fmt.Sprintf("persisted design revision %d %s\n", run.Design.Contract.Revision, run.Design.Hash))
}

func (a application) runApprove(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] != "design" {
		return a.out.usageError("approve requires: design <full-context> <revision> <hash> <actor> [comment]")
	}
	store, execution, rest, err := a.resolve(ctx, args[1:])
	if err != nil {
		return a.stateFailure(err)
	}
	if len(rest) < 3 || len(rest) > 4 {
		return a.out.usageError("approve design requires revision, hash, actor, and optional comment")
	}
	revision, err := strconv.ParseUint(rest[0], 10, 64)
	if err != nil {
		return a.out.usageError("approval revision must be an unsigned integer")
	}
	comment := ""
	if len(rest) == 4 {
		comment = rest[3]
	}
	service, err := a.service(store)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	run, changed, err := service.ApproveDesign(ctx, execution, revision, rest[1], rest[2], comment)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorState, err.Error())
	}
	return a.out.success("approve design", map[string]any{"run": run, "approval": run.Design.Approval, "changed": changed}, "approved exact design revision\n")
}

func (a application) runGraph(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] != "put" {
		return a.out.usageError("graph requires: put <full-context> <path>")
	}
	store, execution, rest, err := a.resolve(ctx, args[1:])
	if err != nil {
		return a.stateFailure(err)
	}
	if len(rest) != 1 {
		return a.out.usageError("graph put requires exactly one contract path after full context")
	}
	data, err := a.deps.FS.ReadFile(rest[0])
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	service, err := a.service(store)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	run, changed, err := service.PutGraph(ctx, execution, data)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorState, err.Error())
	}
	return a.out.success("graph put", map[string]any{"run": run, "graph": run.Construction, "changed": changed}, fmt.Sprintf("persisted work graph revision %d %s\n", run.Construction.Graph.Revision, run.Construction.Hash))
}

func (a application) runConstruction(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return a.out.usageError("construction requires prepare, ready, or integrate")
	}
	action := args[0]
	store, execution, rest, err := a.resolve(ctx, args[1:])
	if err != nil {
		return a.stateFailure(err)
	}
	service, err := a.service(store)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	switch action {
	case "prepare":
		if len(rest) != 1 {
			return a.out.usageError("construction prepare requires a profile path")
		}
		run, plan, changed, err := service.Prepare(ctx, execution, rest[0])
		if err != nil {
			return a.out.failure(ExitFailure, ErrorState, err.Error())
		}
		return a.out.success("construction prepare", map[string]any{"run": run, "plan": plan, "changed": changed}, fmt.Sprintf("prepared %d construction cards\n", len(plan.Cards)))
	case "ready":
		if len(rest) != 0 {
			return a.out.usageError("construction ready accepts only full context")
		}
		cards, err := service.Ready(ctx, execution)
		if err != nil {
			return a.out.failure(ExitFailure, ErrorState, err.Error())
		}
		return a.out.success("construction ready", map[string]any{"context": execution, "cards": cards}, fmt.Sprintf("%d card(s) create-ready\n", len(cards)))
	case "integrate":
		run, result, changed, err := service.Integrate(ctx, execution, rest)
		if err != nil {
			return a.out.failure(ExitFailure, ErrorState, err.Error())
		}
		return a.out.success("construction integrate", map[string]any{"run": run, "integration": result, "changed": changed}, "construction branches integrated\n")
	default:
		return a.out.usageError("construction requires prepare, ready, or integrate")
	}
}

func (a application) runTask(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] != "bind" {
		return a.out.usageError("task requires: bind <full-context> <work-item-id> <task-id>")
	}
	store, execution, rest, err := a.resolve(ctx, args[1:])
	if err != nil {
		return a.stateFailure(err)
	}
	if len(rest) != 2 {
		return a.out.usageError("task bind requires work item ID and task ID")
	}
	service, err := a.service(store)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	binding, created, err := service.BindTask(ctx, execution, rest[0], rest[1])
	if err != nil {
		return a.stateFailure(err)
	}
	return a.out.success("task bind", map[string]any{"binding": binding, "created": created}, "recorded external Kanban task binding\n")
}

func (a application) runWork(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return a.out.usageError("work requires start, complete, or block")
	}
	action := args[0]
	store, execution, rest, err := a.resolve(ctx, args[1:])
	if err != nil {
		return a.stateFailure(err)
	}
	service, err := a.service(store)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	switch action {
	case "start":
		if len(rest) != 1 {
			return a.out.usageError("work start requires a work item ID")
		}
		run, changed, err := service.StartWork(ctx, execution, rest[0])
		if err != nil {
			return a.out.failure(ExitFailure, ErrorState, err.Error())
		}
		return a.out.success("work start", map[string]any{"run": run, "changed": changed}, "work item started with synchronized parents\n")
	case "complete", "block":
		if len(rest) != 4 {
			return a.out.usageError("work complete/block requires work item ID, evidence ID, summary/reason, and command")
		}
		evidence := domain.Evidence{
			Context: execution, ID: rest[1], Kind: "command", Command: rest[3],
			Summary: rest[2], RecordedAt: a.deps.Now().UTC(),
		}
		status, blocker := domain.WorkCompleted, ""
		if action == "block" {
			status, blocker = domain.WorkBlocked, rest[2]
		}
		run, changed, err := service.FinishWork(ctx, execution, rest[0], status, evidence, blocker)
		if err != nil {
			return a.out.failure(ExitFailure, ErrorState, err.Error())
		}
		return a.out.success("work "+action, map[string]any{"run": run, "changed": changed}, "work outcome recorded\n")
	default:
		return a.out.usageError("work requires start, complete, or block")
	}
}

func (a application) runResult(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] != "put" {
		return a.out.usageError("result requires: put <full-context> <phase-result-path>")
	}
	store, execution, rest, err := a.resolve(ctx, args[1:])
	if err != nil {
		return a.stateFailure(err)
	}
	if len(rest) != 1 {
		return a.out.usageError("result put requires exactly one phase-result path")
	}
	data, err := a.deps.FS.ReadFile(rest[0])
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	service, err := a.service(store)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	run, changed, err := service.PutResult(ctx, execution, data)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorState, err.Error())
	}
	return a.out.success("result put", map[string]any{"run": run, "result": run.Construction.Result, "changed": changed}, "construction result persisted\n")
}

func (a application) runResume(ctx context.Context, args []string) int {
	store, execution, rest, err := a.resolve(ctx, args)
	if err != nil {
		return a.stateFailure(err)
	}
	if len(rest) != 0 {
		return a.out.usageError("resume accepts only full context")
	}
	service, err := a.service(store)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	run, changed, err := service.Resume(ctx, execution)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorState, err.Error())
	}
	return a.out.success("resume", map[string]any{"run": run, "changed": changed}, "run resumed without resetting user work\n")
}

func (a application) stateFailure(err error) int {
	code := ErrorState
	switch {
	case errors.Is(err, state.ErrNotRepository):
		code = "not_repository"
	case errors.Is(err, state.ErrNotInitialized):
		code = "not_initialized"
	case errors.Is(err, state.ErrIncompatibleState):
		code = "incompatible_state"
	case errors.Is(err, state.ErrCorruptState):
		code = "corrupt_state"
	}
	return a.out.failure(ExitFailure, code, err.Error())
}

func (a application) runSchema(args []string) int {
	if len(args) != 1 {
		return a.out.usageError("schema requires exactly one contract kind")
	}
	kind, err := contracts.ParseKind(args[0])
	if err != nil {
		return a.out.usageError(err.Error())
	}
	data, err := contracts.Schema(kind)
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, err.Error())
	}
	var schema any
	if err := json.Unmarshal(data, &schema); err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, fmt.Sprintf("decode generated schema: %v", err))
	}
	return a.out.success("schema", map[string]any{
		"kind":   kind,
		"schema": schema,
	}, string(data))
}

func (a application) runValidate(ctx context.Context, args []string) int {
	if len(args) != 6 {
		return a.out.usageError("validate requires a contract kind, path, and full project/feature/run/repository context")
	}
	kind, err := contracts.ParseKind(args[0])
	if err != nil {
		return a.out.usageError(err.Error())
	}
	data, err := a.deps.FS.ReadFile(args[1])
	if err != nil {
		return a.out.failure(ExitFailure, ErrorInternal, fmt.Sprintf("read contract %q: %v", args[1], err))
	}
	_, execution, rest, err := a.resolve(ctx, args[2:])
	if err != nil {
		return a.stateFailure(err)
	}
	if len(rest) != 0 {
		return a.out.usageError("validate received unexpected arguments")
	}
	if err := contracts.ValidateForContext(kind, data, execution); err != nil {
		var validationErrors domainValidationErrors
		if errors.As(err, &validationErrors) {
			return a.out.validationFailure(err.Error(), validationErrors)
		}
		return a.out.failure(ExitFailure, ErrorValidation, err.Error())
	}
	return a.out.success("validate", map[string]any{
		"kind":    kind,
		"path":    args[1],
		"context": execution,
		"valid":   true,
	}, fmt.Sprintf("%s: valid %s contract\n", args[1], kind))
}

// This local interface keeps app's error reporting decoupled from domain types.
type domainValidationErrors interface {
	error
	ValidationDetails() []map[string]string
}

func validateDependencies(deps Dependencies) error {
	switch {
	case deps.Stdin == nil:
		return fmt.Errorf("hermoso: stdin is not configured")
	case deps.Stdout == nil:
		return fmt.Errorf("hermoso: stdout is not configured")
	case deps.Stderr == nil:
		return fmt.Errorf("hermoso: stderr is not configured")
	case deps.FS == nil:
		return fmt.Errorf("hermoso: filesystem is not configured")
	case deps.Version == "":
		return fmt.Errorf("hermoso: version is not configured")
	case deps.Now == nil:
		return fmt.Errorf("hermoso: clock is not configured")
	default:
		return nil
	}
}

func takeJSONFlag(args []string) (bool, []string) {
	filtered := make([]string, 0, len(args))
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return jsonOutput, filtered
}
