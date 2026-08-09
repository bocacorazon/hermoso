# Implementation Plan: Learning DFT as an Agent-Orchestrating-Agent System

## Problem statement

The goal is **not** to replace dft with a separate workflow runner. The goal is to continue developing **dft**, but evolve it from a mostly fixed process supervisor into a **smarter, learning system** where an orchestrator agent selects workflows, invokes specialized skills, validates outputs, learns from failures, and improves the skills it uses.

The earlier docs plan was wrong because it treated the orchestrator as mostly deterministic code and pushed Hermes-style agent behavior to the edge. The intended direction is different:

> dft itself should become an agentic orchestrator of agents.

Given a demand-package, the system should:

1. Recognize what kind of work this is.
2. Select the right workflow family, starting with Spec-Kit.
3. Select the right skill or lane for that workflow.
4. Execute the required tool and agent sequence.
5. Validate each output with lightweight completion checks and content-aware gates.
6. On failure, learn from the outcome and improve the skills.

The thing being optimized is not the flashcard app. It is the **capability of dft to choose, execute, critique, and improve workflows over time**.

## Current codebase state

- `docs/dft.md` captures the current foundation design for dft as a headless workflow engine with strong structure: demand-package, WBS, lanes, phase flows, eval, fix-planner, audit, and git-backed execution.
- `docs/seed-of-a-seed.md` captures the original ambition: AI-native development, recursive dogfooding, Spec-Kit methodology, eval harnesses, and later local-model infrastructure.
- `docs/eval.md` is still only seed-level and does not yet define a serious learning and evaluation framework.
- No implementation exists yet; this repo is still primarily design and research material.

## Reframed vision

DFT should become a **learning orchestration substrate** with four distinct layers:

1. **Orchestrator agent**
   - receives or is triggered by a demand-package
   - classifies the work
   - chooses the workflow family
   - selects or composes the right skill or lane
   - decides how to respond to failure within bounded policies

2. **Worker agents**
   - perform concrete work such as Spec-Kit specify/plan/tasks/implement
   - are guided by reusable skills
   - write artifacts and code

3. **Critic/evaluator agents plus lightweight validators**
   - perform completion checks, analyze gates, and final evals
   - produce explicit findings, not vibes

4. **Learner/improver loop**
   - ingests failures, near-misses, and success cases
   - identifies weak skills, prompt content, validation gaps, or workflow misroutes
   - proposes or applies updates to skills and workflow-selection policy

This is no longer just "a workflow engine with some agent calls." It is a system where **workflow selection and skill evolution are first-class**.

## Pushback

### 0. Should this idea be killed?

No, I do **not** think the whole idea should be killed.

I think there is a **good idea** and a **bad idea** currently entangled:

- **Good idea:** dft becomes an agentic system that chooses workflow families, selects skills, runs validators, routes failures, and improves from evidence.
- **Bad v1 idea:** dft autonomously rewrites and promotes its own skills before you have a strong benchmark, eval, and replay discipline.

So my recommendation is:

> Keep the idea of a learning dft. Kill the idea of unconstrained self-modification in v1.

If you strip out the learning loop entirely, the system becomes much less differentiated. But if you make self-rewriting too autonomous too early, you will get a system that mutates faster than you can trust it.

### 1. If dft becomes agentic, "deterministic orchestrator" is too narrow

That objection is correct. If the orchestrator is fully deterministic, Hermes-like capabilities buy very little. A fully deterministic controller can execute flows, but it cannot really choose workflows, adaptively route failures, or improve its own procedures in a meaningful way.

The better posture is:

- **agentic orchestration**
- with **deterministic boundaries**

That means the system can reason and choose, but:

- side effects are constrained
- skill updates are versioned
- lightweight validators are explicit
- final promotion of learned changes is gated

### 2. Skills cannot just be prompts

If the system is going to learn, a "skill" must be more than text instructions. Otherwise it cannot be inspected, validated, versioned, compared, or safely improved.

A skill should have at least:

- identity and version
- purpose and applicability conditions
- required inputs
- expected outputs
- recommended tool and agent sequence
- completion validators
- evaluation gates
- failure patterns it is meant to handle
- learning metadata describing what evidence can justify changing it

### 3. Learning must target the right artifacts

You do not just want the model to "remember." You want it to improve the **operational artifacts** dft uses:

- workflow-selection policy
- lane-selection policy
- skill contents
- completion validators
- evaluation gates
- remediation strategies
- maybe model-routing policy

If learning is not grounded in these artifacts, the system will look adaptive without actually becoming more capable.

### 4. Eval is even more important in a learning system

Once dft can modify its own skills, eval stops being just release validation. It becomes the control system that decides whether learning is improving the system or corrupting it.

So evaluation has to exist at four levels:

1. **step-level completion validation**
2. **content-evaluation gates during the workflow**
3. **run-level or benchmark-level evaluation**
4. **skill-change evaluation**

### 5. Start with one workflow family

The ambition is general workflow selection later, but the first learning loop should target one workflow family only:

- **Spec-Kit first**
- other workflow families later

That gives the orchestrator a bounded initial domain and makes learning measurable.

### 6. The real kill criteria

I would only recommend killing or drastically cutting this direction if, after the first bounded prototype, one of these becomes true:

1. The orchestrator cannot outperform a static rule-based workflow picker on the benchmark set.
2. Skill updates cannot be evaluated reliably enough to distinguish improvement from noise.
3. Most failures are caused by model weakness or benchmark ambiguity rather than skill or workflow quality.
4. The learner mostly produces prompt churn instead of meaningful operational improvements.

If those happen, the right fallback is not "kill dft." It is:

- keep dft as a strong workflow engine,
- keep skill/version/eval artifacts,
- reduce the scope of autonomous learning.

## Sharpened product definition

DFT remains the product.

Updated purpose:

> DFT is a headless, git-auditable software production system in which an orchestrator agent selects workflows and skills, delegates work to worker agents, validates outputs, and improves the skills and routing policy from observed outcomes.

DFT is still not:

- a chat shell
- a generic personal assistant
- only a deterministic workflow engine

DFT becomes:

- a workflow selector
- a skill selector
- a remediation engine driven by validators and gates
- a learning loop over its own procedures

## Core concepts to add

### 1. Workflow family

A top-level approach for a class of work.

Examples:

- `speckit`
- later: `scaffold`
- later: `migration`
- later: `bugfix`
- later: `research`

The orchestrator agent chooses the workflow family.

### 2. Skill

A reusable operational unit that tells an agent how to carry out a class of tasks.

A skill is not just a prompt. It is a versioned artifact bundle:

- instructions
- context attachments
- tool permissions or toolset assumptions
- expected artifact contract
- companion completion validators
- companion evaluation gates
- improvement history

### 3. Skill graph

For one workflow family, dft should understand which skills are used at which stage.

For Spec-Kit this could initially look like:

- demand-package triage
- workflow selection
- `speckit-specify`
- `speckit-clarify`
- `speckit-plan`
- `speckit-tasks`
- `speckit-analyze`
- `speckit-implement`
- final eval
- fix-planner
- learner

### 4. Failure corpus

A structured store of:

- failing runs
- completion-validator failures
- evaluation-gate findings
- agent transcripts
- artifacts produced
- remediation attempts
- final outcomes

This is the raw material for learning.

### 5. Skill update candidate

A proposed change to a skill, validator, or routing rule generated from evidence in the failure corpus.

It should include:

- target artifact
- reason for change
- evidence references
- proposed diff
- expected benefit
- evaluation plan

## Proposed architecture

### Layer A: Agentic macro-orchestrator

The macro loop should no longer be purely static engine code. It should include an orchestrator agent that operates within explicit boundaries.

Responsibilities:

- ingest demand-package
- classify work type
- choose workflow family
- choose lane or skill set
- decide whether to continue, remediate, recurse, or escalate
- summarize why each choice was made

It should not directly perform arbitrary side effects. It should emit structured decisions consumed by the engine and runtime.

Example structured decision:

```json
{
  "workflow_family": "speckit",
  "lane": "spec",
  "reason": "New greenfield product request with user-facing web scope and clear demand package.",
  "next_action": "run"
}
```

### Layer B: Executable skill-bound workflows

Once a workflow family is chosen, dft should run a workflow whose stages are backed by skills.

For the initial Spec-Kit family:

- specify
- clarify
- plan
- tasks
- analyze
- implement
- evaluate
- remediate

Each stage should have:

- a worker skill
- lightweight completion validators
- optional content-evaluation gates
- explicit output artifacts
- retry and remediation semantics

### Layer C: Completion validators and evaluation gates

These need to be split into **two different mechanisms**.

#### C1. Lightweight completion validators

These do **not** judge whether the content is good. They only answer:

> Did the agent run actually complete the step, or did it leave behind an obviously incomplete artifact?

Examples:

- artifact file exists
- required sections exist
- template placeholders were replaced
- `sha(template) != sha(artifact)`
- artifact is not trivially identical to the template
- required output parses
- step emitted the expected artifact set

These checks should be cheap and structural. Their purpose is to catch cases where the agent technically "ran" but did not actually do the work.

#### C2. Content-evaluation gates

Some steps produce outputs that need semantic judgment, not just structural checks.

Examples:

- `analyze` findings
- final benchmark eval
- selected plan or task quality checks where content matters materially

These are decision points that can alter control flow.

Example:

- if `analyze` returns critical or important findings, route back to `tasks`
- then regenerate downstream artifacts as needed

So a skill or stage should declare both:

- **completion validators**: cheap structural checks
- **evaluation gates**: semantic checks that may trigger remediation or rerouting

### Layer D: Learning system

The learner consumes evidence and proposes improvements to:

- skills
- completion validators
- evaluation gates
- routing rules
- lane assignment heuristics
- maybe model-selection policy

The learner should not operate on freeform memory alone. It should produce concrete, reviewable artifact changes.

## Learning loop design

### Input signals

- failed completion validators
- evaluation-gate findings
- failed final evals
- repeated remediation loops
- malformed outputs
- unnecessary human intervention
- benchmark comparisons showing regressions
- success cases with markedly better outcomes

### Learning outputs

- skill patch proposals
- completion-validator patch proposals
- evaluation-gate patch proposals
- workflow-selection rule updates
- lane-selection rule updates
- benchmark additions

### Learning stages

1. **Observe**
   - capture transcripts, outputs, validator findings, and metrics
2. **Diagnose**
   - identify whether failure came from workflow selection, skill quality, validator weakness, model mismatch, or benchmark ambiguity
3. **Propose**
   - produce a candidate skill, rule, or validator update
4. **Evaluate**
   - rerun against benchmark set or targeted failure cases
5. **Promote**
   - if accepted by policy, make the updated artifact active

## Recommended v1 cut

To keep this idea sane, v1 should include:

- orchestrator agent chooses workflow family and lane
- worker skills execute the Spec-Kit family
- lightweight completion validators catch incomplete agent runs
- content-evaluation gates control remediation
- learner produces **skill-update candidates**
- replay and benchmark system test those candidates
- promotion of updates is conservative

V1 should exclude:

- freeform autonomous rewriting of active skills with no replay gate
- multiple workflow families from day one
- generalized self-improvement across every dft artifact at once
- uncontrolled memory-based adaptation with no artifact diff

This preserves the core insight while keeping the system falsifiable.

## Initial target architecture for dft

### Repository-side artifacts

Likely additions under `.dft/`:

- `.dft/workflow-families/`
- `.dft/skills/`
- `.dft/completion-validators/`
- `.dft/evaluation-gates/`
- `.dft/evals/`
- `.dft/learning/`
- `.dft/failure-corpus/`
- `.dft/runs/`
- `.dft/context/`

### Skill artifact shape

For v1 planning purposes, assume a skill artifact contains:

- metadata
- instructions or prompt contract
- required context
- allowed tools or toolset
- expected outputs
- completion-validator references
- evaluation-gate references
- examples
- history or version notes

### Runtime state

Per run, capture:

- orchestrator decisions
- chosen workflow family
- chosen skills
- worker outputs
- completion-validator results
- evaluation-gate results
- remediation history
- learning candidates spawned from the run

## Implementation phases

### Phase 0: Reframe dft identity and architecture

Deliverables:

- replace the current mental model of dft as only a deterministic process supervisor
- write a revised product brief: "learning dft"
- define the split between:
  - orchestrator agent
  - worker agents
  - completion validators
  - evaluation gates
  - learner
- define v1 non-goals so the project does not explode

### Phase 1: Define workflow-family and skill abstractions

Deliverables:

- workflow-family schema
- skill schema
- completion-validator schema
- evaluation-gate schema
- routing-decision schema
- skill-update-candidate schema
- failure-corpus schema

### Phase 2: Define the first workflow family: Spec-Kit

Deliverables:

- initial `speckit` workflow-family definition
- skill graph for:
  - specify
  - clarify
  - plan
  - tasks
  - analyze
  - implement
  - eval
  - fix-planner
  - learner
- output contracts for each stage
- lightweight completion validators for each stage
- content-evaluation gates for steps like `analyze` and final eval
- explicit remediation edges, including `analyze -> tasks`

### Phase 3: Define the benchmark and eval system

Deliverables:

- benchmark manifest schema
- flashcard benchmark manifest
- verdict schema
- evidence bundle schema
- step-level completion-validator definitions
- content-evaluation gate definitions
- final-eval rubric definitions
- benchmark comparison format

### Phase 4: Build the agentic orchestration layer

Deliverables:

- orchestrator decision interface
- runtime that executes orchestrator decisions safely
- workflow-family selector
- skill selector
- bounded remediation and retry policy
- escalation path for unresolved failures

### Phase 5: Build the worker and validator execution loop

Deliverables:

- stage execution runtime
- skill invocation runtime
- completion-validator runtime
- evaluation-gate runtime
- artifact capture
- transcript capture
- structured result capture
- step-to-step handoff rules
- remediation routing rules such as `analyze -> tasks`

### Phase 6: Build the learning loop

Deliverables:

- failure-corpus ingestion
- diagnosis pass over failed runs
- skill update candidate generation
- validator and gate update candidate generation
- targeted replay against benchmark cases
- promotion policy for successful updates

### Phase 7: Run the first end-to-end learning cycle

Deliverables:

- demand-package intake
- orchestrator picks `speckit`
- flashcard app developed through the Spec-Kit family
- lightweight completion validators executed
- analyze and final evaluation gates executed
- if failure: remediation and learning candidate generated
- if pass: still mine the run for improvement opportunities

### Phase 8: Dogfood on dft improvements

Deliverables:

- express a dft enhancement as a demand-package
- let dft route and execute its own improvement workflow
- compare before and after skill versions
- measure whether learning improved results

### Phase 9: Expand beyond Spec-Kit

Deliverables:

- second workflow family
- workflow-family routing improvements
- generalized skill selection policy
- richer learner capable of cross-family insights

## First milestones

The first milestone should be:

> Given a demand-package, dft's orchestrator agent chooses the `speckit` workflow family, runs the corresponding skill-backed stages, uses lightweight completion validators to catch incomplete agent runs, uses content-evaluation gates where meaning matters, and produces a complete evidence bundle plus final verdict.

The second milestone should be:

> When a run fails, dft produces a concrete skill-update candidate or validator-update candidate grounded in failure evidence.

## Risks

- The system may become "agentic theater" if skills are not versioned artifacts.
- The learner may produce noisy or harmful updates without a strong evaluation and promotion policy.
- Workflow selection may look intelligent but actually just encode one hardcoded path unless alternative workflow families are introduced.
- The benchmark and eval system may be too weak to distinguish real skill improvement from overfitting.

## Key unresolved decision

The main remaining design choice is **how autonomous skill evolution should be**:

- fully propose-only
- auto-apply into a staging area after benchmark replay
- auto-promote after replay without human review

That decision controls how aggressive the learning loop can be in v1.
