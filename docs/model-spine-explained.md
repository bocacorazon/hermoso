# Model Spine: Code Intelligence in Hermoso

Reference: explanation of the "model spine" paragraph from `docs/verification-loop-critique.md`.

## Overview

The "model spine" is Hermoso's code intelligence system, living in `internal/model/`.
It builds a structured snapshot of what the candidate repository looks like at a given
git commit. The critique calls it a "spine" because it's the backbone of evidence that
verification operates on — every surface resolution, every test pairing, every component
boundary is derived from it.

## The Six Pieces

### 1. Tree-sitter Extraction (`tree_sitter.go`)

Go parses source files using Tree-sitter grammars for Go, JavaScript, TypeScript, and
Python. It extracts structural nodes — functions, types, interfaces, methods — with
their source ranges and attributes. Each node records its producer as
`tree-sitter-<language>/v1`. This is deterministic: same source file, same parse tree,
same nodes.

### 2. Git Inventory (`inventory()` in `build.go`, line 154)

Go runs `git ls-tree -r -l -z --full-tree <revision>` to enumerate every tracked file
in the candidate commit. For each file it records the git object hash, size, language
(by extension), whether it's a test file, and content hash. Files over 2MB are hashed
but not read into memory. This produces the `FileFact` list that feeds all downstream
extractors.

### 3. Component Aggregation (`addComponents()`, line 237)

Go groups files by their top-level directory into `component:` nodes. This is a simple
deterministic grouping — no reasoning needed, just path inspection. Each component node
links to its member files via `contains` edges.

### 4. Test Pairing (`addTestRelationships()`, line 278)

Go infers which test file exercises which source file using naming conventions (e.g.,
`foo_test.go` exercises `foo.go`). It creates `exercises` edges between `test:` and
`file:` nodes. Again, deterministic — it's a naming-convention heuristic, not a
semantic analysis.

### 5. SCIP Augmentation (`scip.go`)

If a SCIP (Source Code Intelligence Protocol) index is provided, Go decodes it and
augments the graph with symbol occurrences — definitions and references with precise
source ranges. This adds `symbol:scip:` nodes with cross-reference edges. The SCIP
index must come from an external tool (like scip-go or scip-typescript), but the
augmentation itself is deterministic: decode protobuf, validate paths against the git
tree, add nodes.

### 6. Incremental Reuse (`reusablePaths()` + `reuseUnchangedNodes()` + `reuseUnchangedEdges()`, lines 343-433)

When rebuilding a model snapshot for a new commit, Go compares file content hashes
against the previous snapshot. Files whose content hash hasn't changed skip
re-extraction entirely — their Tree-sitter nodes and edges are reused from the
previous snapshot. SCIP data is reused if the SCIP input hasn't changed. This makes
re-verification fast when only a few files changed.

## Query Interface (`query.go`)

Go provides four query modes over a snapshot:

- **orientation** — high-level overview (repository, components, interfaces)
- **task** — find nodes matching search text
- **impact** — BFS from given node IDs to find related nodes
- **evidence** — fetch specific nodes by ID

Results are budget-limited (default 12KB) and rendered as markdown. This is what a
skill would call to ask "what's in this repository?" or "what does this change
affect?".

## Why the Critique Says Go Owns It

The principle is: if removing the model makes the result MORE trustworthy, it should
be Go. All six pieces above are deterministic — same input always produces same
output. There's no judgment involved:

- Tree-sitter parsing is a fixed grammar, not a judgment call
- Git inventory is a mechanical tree walk
- Component grouping is path-based, not semantic
- Test pairing is naming-convention matching
- SCIP augmentation is protocol decode + validation
- Incremental reuse is content-hash comparison

A model doing any of these would make them LESS trustworthy — the model could misparse
code, skip files, group things inconsistently, or hallucinate symbol relationships.
So Go owns all of it.

## Where the Skill Comes In

The skill doesn't build or query the model directly. Instead, Go builds the snapshot,
persists it (`PutModelSnapshot`), and the skill can query it later. The one place where
the critique flags a problem (Issue C) is `resolveCandidateSurfaces` (line 442) —
that's where Go uses the model snapshot to resolve "planned" surfaces, and it does so
with a fuzzy title match (`strings.EqualFold(node.Title, surface.Title)`). That IS a
judgment task the model would do better, but it's the only part of the model spine the
critique flags as misplaced.

The rest — the extraction, aggregation, pairing, SCIP, reuse, and query — all correctly
live in Go.

## File References

| File | Purpose |
|---|---|
| `internal/model/build.go` | Orchestrates the full model build: inventory, extraction, aggregation, reuse |
| `internal/model/tree_sitter.go` | Tree-sitter grammar loading and structural node extraction |
| `internal/model/scip.go` | SCIP index decoding and graph augmentation |
| `internal/model/query.go` | Query engine: orientation, task, impact, evidence modes |
| `internal/model/render.go` | Renders markdown views (system context, components, testing, etc.) |
| `internal/workflow/verification.go` | Calls `model.Build()` to build candidate snapshot during verification |
| `internal/workflow/publication.go` | Calls `model.Build()` to build published snapshot after pass |

## Key Types

- `BuildRequest` — input to `Build()`: project, repo root, revision, SCIP path, previous snapshot
- `FileFact` — per-file metadata from git inventory (path, git object, size, content hash, language, test flag)
- `graph` — internal mutable graph (nodes map + edges map) during construction
- `QueryRequest` / `QueryResult` — query interface for skills to inspect a snapshot
- `ModelSnapshot` — the finalized, hashed, immutable output (nodes, edges, views, manifest)
