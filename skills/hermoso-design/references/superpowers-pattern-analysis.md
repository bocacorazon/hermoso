# Design Skill Pattern Analysis — Superpowers Comparison

Analysis of the [superpowers](https://github.com/obra/superpowers) skill
library (brainstorming, writing-plans, subagent-driven-development) conducted
when designing the v3.0.0 path-branching rewrite of `hermoso-design`.

## What was borrowed

### 1. Path classification ("choose your own adventure")

Superpowers' `brainstorming` skill classifies requests into three paths
(spike / bounded / architectural) BEFORE any work, and announces the
classification so the user can override it. Each path has a distinct checklist
and terminal state — not just variable depth on the same linear process.

Hermoso's v3.0.0 adapts this to three paths: **bounded / standard / complex**.
The spike path was skipped because Hermoso already has a dedicated `spike`
skill for feasibility probes. The classification heuristics are Hermoso-specific
(bounded = existing flow in the repo to change; complex = cross-package, new
abstractions, domain modeling).

### 2. The HARD GATE

Superpowers has an explicit, capitalized, non-negotiable gate: no
implementation action until the user has seen and approved the design. The
gate language is assertive enough to stop momentum.

Hermoso's version maps this to: no `hermoso design put`, no `hermoso
verification put`, no construction dispatch until the user approves the
presented design. This was the direct fix for a real failure mode observed
on the gchat-second-brain project — the agent went from prompt to `design put`
without presenting anything for approval.

### 3. The one-way ratchet

Superpowers: "hidden complexity discovered mid-task upgrades the path — stop,
say so, and step up. Nothing downgrades mid-task."

Hermoso adopted this verbatim. If a bounded feature turns out to be
cross-package, the skill explicitly instructs: stop, announce the upgrade,
re-classify.

### 4. Red flags table

Superpowers includes a "Red Flags" table mapping rationalized thoughts to
reality. This is a strong pattern for preempting the agent's own
self-justification. Hermoso adapted the table entries to its own path names.

## What was consciously NOT borrowed

### Spike path

Superpowers has a spike path (feasibility probe, throwaway code, no design
doc). Hermoso already has a separate `spike` skill for this, so it was
redundant. The user confirmed: "skip spike — Hermoso already has the spike
skill for that."

### In-chat-only design (no contract)

Superpowers' bounded path produces a short design in chat and proceeds
directly to implementation — no spec file, no formal contract. Hermoso's
bounded path still requires the feature-design JSON and verification contract
because the Hermoso lifecycle depends on machine-validated contracts for
hash-locked design packages and verification. The contract can be
lightweight, but it cannot be omitted.

### Subagent-driven development

Superpowers dispatches a fresh subagent per task with two-stage review (spec
compliance, then code quality). Hermoso uses Kanban-based dispatch instead —
work items become Kanban cards become workers. This is a fundamental
architectural difference: Hermoso's construction phase IS its
subagent-driven-development equivalent, but with persistent state, worktree
management, and deterministic graph validation that ad-hoc subagent
dispatch doesn't provide. The user explicitly confirmed: "I like the
kanban-as-orchestration."

### Writing-plans skill (separate plan document)

Superpowers has a separate `writing-plans` skill that produces a detailed
implementation plan with bite-sized TDD steps before execution. Hermoso's
construction skill produces a construction plan (`construction-plan.md`) and
a validated work graph, but the decomposition is driven by the approved
design contract's requirements/surfaces/criteria rather than by writing
free-form plan steps. The work graph IS the plan, and Go validates its
completeness (every requirement covered, every surface covered, traceability
rules enforced).

## Origin session

This analysis was conducted in the session where the user said: "I had
something more in line with superpowers, where the phases in the workflow
have variants adapting to the project in 'choose your own story' style." The
user's pain point was that on the gchat-second-brain project, "the agent went
from prompt straight to execution — no exploration of the idea, clarification,
presentation of the plan for approval." The v3.0.0 rewrite addresses this
directly through path classification, the hard gate, and the ratchet.
