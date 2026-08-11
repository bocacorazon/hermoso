---
status: proposal
epistemic_status: working
audience: maintainers
document_type: explanation
---

# Repository Knowledge Spine

## Summary

Hermoso should provide every skill and worker with a compact, navigable model of
the target repository. The model should orient an agent quickly, identify where
deeper evidence lives, and support impact analysis without copying the repository
into generated prose.

This proposal calls that model the **repository knowledge spine**. It is a
clean-room design derived from public code-intelligence, knowledge-graph,
architecture-description, and information-organization frameworks.

The spine is:

- an index over source facts, relationships, architecture views, and decisions;
- progressively disclosed, from project orientation to exact source evidence;
- incrementally refreshed from Git and content hashes;
- explicit about provenance and epistemic status;
- scoped by Hermoso's canonical project identity.

It is not:

- a replacement for source code, documentation, or code search;
- a collection of ungrounded LLM summaries;
- one supposedly authoritative decomposition of the system;
- a mandatory up-front documentation project before feature work can begin;
- derived from any employer-owned schema, prompts, taxonomy, or implementation.

## Motivation

Agents currently rediscover repository structure independently. This is expensive,
inconsistent, and risky when several projects and feature runs are active. A shared
spine should answer four initial questions:

1. What system am I working in?
2. Which part of it is relevant to this task?
3. What depends on that part or is affected by changing it?
4. Which source evidence supports this understanding?

Hermoso already provides an unambiguous `project_id`, `feature_id`, `run_id`, and
canonical repository identity. The spine extends that identity boundary with
repository knowledge; it must never infer the active project from conversation or
the process working directory.

## Public foundations

### Deterministic code intelligence

- [Tree-sitter](https://tree-sitter.github.io/tree-sitter/) provides incremental
  syntax trees across many languages. It is suitable for files, declarations,
  imports, and language-specific structural facts.
- [SCIP](https://github.com/sourcegraph/scip) provides stable cross-file symbol
  identities, definitions, references, and relationships using compiler-aware
  indexers.
- [Code Property Graphs](https://joern.io/) combine syntax, control flow, and data
  flow when deeper program analysis is justified.

These systems should supply observed facts. An LLM should not reconstruct facts
that a parser, compiler, or version-control system can report.

### Ranked repository orientation

[Aider's repository map](https://aider.chat/docs/repomap.html) demonstrates a
useful navigation pattern:

1. extract declarations and references;
2. construct a relationship graph;
3. rank relevant files and symbols;
4. render a compact map within a context budget.

Hermoso should adopt the principle rather than copy Aider's representation: rank
and render the smallest useful orientation view for the current task.

### Hierarchical graph summaries

[Microsoft GraphRAG](https://microsoft.github.io/graphrag/) demonstrates
hierarchical graph communities and summaries at multiple levels. For a repository,
similar clustering can produce views ranging from system-level domains down to
related packages or symbols.

Graph clustering is advisory. Explicit module boundaries, manifests, ownership,
and architecture declarations take precedence over statistically inferred
communities.

### Architecture knowledge

- [C4](https://c4model.com/) supplies explicit abstraction levels: system context,
  containers, components, and code.
- [arc42](https://arc42.org/) supplies practical viewpoints including constraints,
  building blocks, runtime, deployment, cross-cutting concepts, risks, and
  decisions.
- [Architecture Decision Records](https://github.com/architecture-decision-record/architecture-decision-record)
  preserve context, decisions, alternatives, and consequences.
- [DDD context maps](https://martinfowler.com/bliki/BoundedContext.html) describe
  named domain boundaries and their integration relationships.

The spine should link to existing architecture material and decisions rather than
generate competing canonical documents.

### Knowledge organization and provenance

- [SKOS](https://www.w3.org/TR/skos-reference/) offers a lightweight vocabulary
  model with broader, narrower, and related concepts.
- [W3C PROV](https://www.w3.org/TR/prov-dm/) separates entities, activities, and
  agents, providing a useful conceptual model for provenance.
- [Information Foraging Theory](https://www.nngroup.com/articles/information-foraging/)
  motivates strong information scent: compact labels and summaries that indicate
  where valuable detail can be found.
- [Diataxis](https://diataxis.fr/) distinguishes tutorials, how-to guides,
  reference, and explanation.

Hermoso does not need RDF, OWL, or a general-purpose ontology reasoner. These
frameworks inform a small versioned vocabulary and explicit provenance fields.

## Layered model

The spine separates three kinds of knowledge.

### 1. Observed facts

Deterministically extracted:

- repository, commit, files, and languages;
- modules, packages, symbols, declarations, and references;
- imports and dependency manifests;
- executable entry points;
- API/schema declarations;
- tests and the source surfaces they exercise;
- build, lint, test, and deployment commands;
- ownership and existing documentation;
- source locations and content hashes.

Observed facts carry no agent confidence score. They carry an extractor, extractor
version, source revision, and evidence location.

### 2. Derived structure

Computed from observed facts:

- dependency and reference graphs;
- strongly connected components and graph communities;
- likely subsystem or component boundaries;
- high-centrality files and symbols;
- change-impact neighborhoods;
- uncovered or weakly documented surfaces;
- ranked orientation maps for a task.

Derived claims record the algorithm and exact input snapshot. They remain
recomputable and must not be presented as authored architecture decisions.

### 3. Interpretive knowledge

Authored by people or agents:

- system and component purpose;
- domain language and invariants;
- architecture boundaries;
- runtime and data-flow explanations;
- conventions and constraints;
- risks, tradeoffs, and design decisions;
- operational knowledge.

Every interpretive item links to supporting facts, documents, or decisions and
declares an epistemic status:

```text
draft -> working -> stable -> canonical
                  \-> deprecated -> archived
```

`canonical` means explicitly accepted by project policy; it is never assigned only
because an agent generated a confident summary.

## Required facets

Each spine node has a small common vocabulary:

| Facet | Examples |
| --- | --- |
| `kind` | repository, file, symbol, component, interface, test, command, concept, invariant, requirement, scenario |
| `abstraction` | system, container, component, code |
| `aspect` | structure, runtime, data, API, testing, operations, security, vocabulary |
| `epistemic_status` | observed, derived, draft, working, stable, canonical, deprecated |
| `audience` | agent, developer, operator, user |
| `document_type` | tutorial, how-to, reference, explanation |

The vocabulary is itself a versioned spine artifact. Projects may extend it without
changing the core contract.

## Artifact layout

The initial persisted representation should be simple, diffable, and
tool-independent:

```text
.hermoso/model/
├── current.json
├── snapshots/<snapshot-id>/
│   ├── manifest.json
│   ├── vocabulary.json
│   ├── nodes.jsonl
│   ├── edges.jsonl
│   ├── index.md
│   └── views/
│       ├── system-context.md
│       ├── domains.md
│       ├── components.md
│       ├── runtime.md
│       ├── data.md
│       ├── interfaces.md
│       ├── testing.md
│       ├── operations.md
│       ├── decisions.md
│       └── vocabulary.md
└── cache/
```

As with other `.hermoso` state, generated model artifacts remain clone-local by
default. Projects may deliberately promote selected views into tracked
documentation.

### Verification-contract and scenario facet

Feature designs reference one immutable snapshot and use its interface,
testing, vocabulary, and invariant nodes. Planned interaction surfaces remain a
design overlay until candidate verification resolves them to observed nodes.

Approved Gherkin stays sealed during construction. After a passing verification
report, Hermoso commits only its hash-locked publication paths, rebuilds the
snapshot at that publication commit, and adds `scenario` nodes with `verifies`,
`uses_concept`, `exercises`, and `governed_by` edges. Verified scenario
knowledge becomes `stable`; `canonical` remains an explicit project-policy
decision.

### Manifest

```json
{
  "schema_version": "hermoso-repository-model/v1",
  "project_id": "project-...",
  "repository": "/canonical/repository/path",
  "source_revision": "<git-commit>",
  "vocabulary_version": "v1",
  "generated_at": "2026-08-02T00:00:00Z",
  "extractors": [
    {"name": "git", "version": "..."},
    {"name": "tree-sitter", "version": "..."},
    {"name": "scip", "version": "..."}
  ]
}
```

### Node

```json
{
  "id": "symbol:scip-package main/MyFunction().",
  "kind": "symbol",
  "abstraction": "code",
  "aspects": ["structure", "runtime"],
  "title": "MyFunction",
  "summary": "Coordinates one bounded operation.",
  "epistemic_status": "observed",
  "evidence": [
    {
      "path": "internal/example/example.go",
      "start_line": 20,
      "end_line": 48,
      "content_hash": "sha256:..."
    }
  ],
  "producer": {"kind": "extractor", "name": "scip-go", "version": "..."},
  "derived_from": []
}
```

### Edge

```json
{
  "source": "component:workflow",
  "relation": "depends_on",
  "target": "component:state",
  "epistemic_status": "derived",
  "evidence": ["symbol:..."],
  "producer": {"kind": "algorithm", "name": "dependency-clustering", "version": "v1"}
}
```

## Navigation behavior

The spine should support four bounded retrieval modes:

### Orientation

Return the smallest system/domain/component map needed to begin work. This is the
default entry point for a new agent or phase skill.

### Task scope

Given the current Hermoso context and feature objective, rank relevant components,
files, symbols, tests, decisions, and commands within a context budget.

### Impact

Given one or more changed nodes, traverse incoming/outgoing relationships to find
likely affected interfaces, consumers, tests, documents, and operational surfaces.

### Evidence

Resolve any summary or relationship back to exact source locations, commits,
documents, decisions, and producer metadata.

Agents should request a deeper view rather than loading the complete spine.

## Incremental maintenance

Each update begins from a Git revision and content hashes:

1. identify changed, added, moved, and deleted files;
2. rerun affected deterministic extractors;
3. update changed nodes and edges;
4. traverse reverse provenance/dependency edges;
5. mark affected derived and interpretive summaries stale;
6. regenerate only stale summaries;
7. preserve accepted human-authored knowledge unless its evidence changed;
8. atomically publish a new manifest revision.

Moving a file should not invalidate a compiler-level symbol identity when SCIP or
another indexer can preserve it. Otherwise Hermoso records an explicit replacement
relationship.

Freshness is evidence-based. Timestamps alone do not establish that a summary is
current.

## Hermoso integration

The spine should be a repository service, not another mandatory development phase.

### Before design

The `hermoso` controller requests an orientation view and a task-scoped map. The
design skill uses these as evidence but may inspect source directly.

### During construction

Each Kanban card receives:

- the canonical Hermoso project/feature/run context;
- a bounded task-scoped spine view;
- relevant decisions and invariants;
- source and test entry points;
- the source revision used to produce the view.

Workers block or refresh when the supplied model belongs to another project or is
stale relative to required parent changes.

### After construction

Changed facts are re-indexed. Interpretive summaries are regenerated only when
their evidence changed. Accepted design decisions may be proposed for promotion
into tracked ADRs, but promotion remains explicit.

### Future verification

Verification can compare:

- expected versus observed changed surfaces;
- documented versus actual interfaces;
- impacted tests versus executed evidence;
- canonical invariants versus implementation behavior;
- stale or contradicted interpretive knowledge.

## Clean-room constraints

Implementation must be based only on this proposal, public documentation, published
papers, and permissively licensed source selected after license review.

Do not use or reproduce:

- employer-owned prompts, schemas, templates, aspect taxonomies, examples, tests,
  naming, output formats, or agent decomposition;
- recollections of non-public implementation details;
- outputs copied from the employer system and paraphrased into fixtures.

Before implementation:

1. preserve this independently derived proposal and its public references;
2. write Hermoso-specific acceptance criteria without comparing output to the
   employer system;
3. record licenses and provenance for adopted dependencies;
4. use independently authored fixtures from public or synthetic repositories.

## Recommended first slice

The first implementation should prove navigation value before adding agent-generated
architecture:

1. define `manifest`, `node`, `edge`, and vocabulary contracts;
2. index Git files and Tree-sitter symbols;
3. consume SCIP when an indexer is available;
4. create import/reference edges;
5. rank a compact Aider-style orientation map;
6. expose `hermoso model build`, `status`, `query`, and `explain`;
7. inject a bounded model view into one design run;
8. evaluate navigation accuracy, freshness, token cost, and source traceability.

GraphRAG summaries, component inference, Code Property Graph analysis, and automatic
documentation promotion should follow only if the deterministic slice proves useful.

## Evaluation criteria

- **Identity:** no result can cross project boundaries.
- **Traceability:** every fact or interpretation resolves to evidence and producer.
- **Freshness:** changed evidence invalidates dependent knowledge.
- **Navigation:** agents locate relevant source with fewer exploratory reads.
- **Compression:** orientation fits a declared context budget.
- **Recall:** important interfaces, tests, decisions, and dependents are not omitted.
- **Precision:** task-scoped views avoid unrelated repository content.
- **Reproducibility:** deterministic layers reproduce from the same revision.
- **Honesty:** derived and interpretive claims are distinguishable from observed facts.
- **Replaceability:** extractors, rankers, and summarizers can change without changing
  the core model contract.

## Open decisions

- Whether JSONL remains sufficient or an embedded graph database is warranted.
- Which languages receive first-class Tree-sitter and SCIP support initially.
- How project-specific vocabulary extensions are reviewed and versioned.
- Whether accepted interpretive views live only under `.hermoso/model` or can be
  promoted into tracked `docs/`.
- Which benchmark repositories and tasks measure navigation improvements.
- What context budget and ranking policy should be the default for Hermes workers.
