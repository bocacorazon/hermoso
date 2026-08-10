# Dependency provenance

Hermoso pins the following upstreams for repository modeling and verification
contract parsing:

| Dependency | Pinned module | Purpose | License |
| --- | --- | --- | --- |
| Tree-sitter Go binding | `github.com/tree-sitter/go-tree-sitter v0.25.0` | Incremental syntax-tree parsing | MIT |
| Go grammar | `github.com/tree-sitter/tree-sitter-go v0.25.0` | Go structure | MIT |
| JavaScript grammar | `github.com/tree-sitter/tree-sitter-javascript v0.25.0` | JavaScript structure | MIT |
| TypeScript/TSX grammar | `github.com/tree-sitter/tree-sitter-typescript v0.23.2` | TypeScript and TSX structure | MIT |
| Python grammar | `github.com/tree-sitter/tree-sitter-python v0.25.0` | Python structure | MIT |
| SCIP Go bindings | `github.com/scip-code/scip/bindings/go/scip v0.9.0` | Optional compiler-grade symbol/reference ingestion | Apache-2.0 |
| Cucumber Gherkin parser | `github.com/cucumber/gherkin/go/v42 v42.0.1` | Parse and validate Gherkin; not a step runner | MIT |
| Cucumber messages | `github.com/cucumber/messages/go/v34 v34.2.0` | Gherkin AST/message types | MIT |
| Go protobuf runtime | `google.golang.org/protobuf v1.36.11` | Decode SCIP protobuf data | BSD-3-Clause |

Versions are recorded in `go.mod`/`go.sum`. Tree-sitter uses CGO and requires a
C compiler. SCIP ingestion is explicit and optional. Hermoso intentionally does
not depend on Godog or implement a new Gherkin step-definition runtime; approved
contracts invoke an existing repository runner or sealed probe harness.
