# Construction Plan: hermoso-constitution

## Decomposition Strategy

The feature decomposes into 4 independent skill-file work items plus 1 integration item. All items modify files under `skills/` and `profiles/` — no Go code changes. The items are:

1. **Write the new `hermoso-constitution` skill** — standalone, no dependencies on other items
2. **Add constitution gate to `hermoso-design`** — standalone, modifies existing skill
3. **Add constitution gate to `hermoso-construction`** — standalone, modifies existing skill
4. **Update `skills_test.go` + `profiles/default.yaml`** — standalone, adds the new skill to the test array and profile binding

Item 5 (integration) depends on all four: it runs the skill tests, verifies all probes pass, and records the construction result.

## Dependency Analysis

- Items 1-4 are independent — no parent links, can run in parallel
- Item 5 (integration) depends on 1, 2, 3, and 4

Critical path: max(item1, item2, item3, item4) → item5

## Parallelization

Items 1-4 can all run in parallel. Each touches a different file:
- Item 1: `skills/hermoso-constitution/SKILL.md` (new)
- Item 2: `skills/hermoso-design/SKILL.md` (modify)
- Item 3: `skills/hermoso-construction/SKILL.md` (modify)
- Item 4: `skills/skills_test.go` + `profiles/default.yaml` (modify)

Item 5 (integration) runs after all four complete: runs `go test ./skills/ -v` and verifies the probe scripts pass.

## Integration Points

Single fan-in: item 5 merges results from items 1-4 and runs the full test suite.

## Risk Areas

- **Skill test invariants** — the new skill must pass all 5 tests. The new skill needs `.hermoso` prohibition, `never infer` + `hermoso context` + `explicit` language, valid frontmatter, and related_skills that resolve. The `TestCommandLabelsMatchCurrentCLI` test checks that current CLI commands appear across all skills combined — the new skill doesn't need to add commands but must not remove coverage.
- **`TestDefaultProfileBindings`** — the profile must list `hermoso-constitution` as a skill string. Item 4 handles this.
- **Frontmatter format** — the new skill's `related_skills` must all resolve to known skill names. The skillNames array in `skills_test.go` must include `hermoso-constitution` (item 4).
