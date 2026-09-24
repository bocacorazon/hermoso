# Construction Plan — graduated-verification

## Decomposition strategy

Seven work items in five waves, decomposed by package ownership:
- Constitution and skills are text-only changes (independent, Wave 1)
- Domain + workflow for attempt cap are tightly coupled (Wave 1)
- Derivation engine and CLI flag build on the domain/workflow (Waves 2–3)
- Probes validate the whole stack (Wave 4)
- Integration fans in all leaves (Wave 5)

## Dependency analysis

```
Wave 1 (parallel): constitution-amend | max-attempts-domain-workflow | skill-updates
Wave 2:               derivation-engine (needs domain types from max-attempts)
Wave 3:               cli-derive (needs derivation-engine + workflow changes)
Wave 4:               probes (needs all Go changes done)
Wave 5:               integration (fan-in all leaves)
```

Critical path: max-attempts → derivation-engine → cli-derive → probes → integration

## Parallelization

Wave 1 items run in parallel: constitution, domain/workflow, and skills share no files.

## Integration points

Single fan-in at integration: `go test ./... -count=1` + `go test ./skills/ -v` + build binary. No multi-leaf merge conflicts expected — each item touches distinct files.

## Risk areas

- **derivation-engine**: new code, most design uncertainty. The DeriveVerification function must correctly map work-graph validation_commands to judgments and handle coverage gaps.
- **max-attempts ripple**: the hardcoded `2` appears in ~6 sites across 3 files. Missing one site means existing tests pass but the feature is incomplete.
- **probes**: stub→real conversion requires the features to actually work end-to-end. Late in the wave, catches integration issues.

## Implementation approach

Orchestrator-direct — all work items implemented by me in sequence, committing to the feature worktree. No kanban dispatch for this feature (it's a Go CLI change in the main hermoso repo, and the construction profile already uses the same model I'm on).