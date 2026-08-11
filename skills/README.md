# Hermoso skills

These source-controlled skills are loaded from this repository. They are not
copied into target repositories and they never install or edit target
`.hermoso` state.

The lifecycle skills are `hermoso`, `hermoso-design`,
`hermoso-verification-author`, `hermoso-construction`, and
`hermoso-verification`. The verification-author and verifier are separate roles
so construction contexts never receive hidden contract assets.

## One-time Hermes setup

Add this repository's `skills` directory to the user's existing Hermes config:

```yaml
skills:
  external_dirs:
    - /absolute/path/to/hermoso/skills
```

This is a manual, one-time user configuration change. Repository tests and the
doctor script never modify `~/.hermes/config.yaml`.

Then select or replace the bindings in `profiles/default.yaml`, or point
`HERMOSO_PROFILE` at a compatible JSON profile document. The checked-in
`.yaml` file intentionally uses JSON syntax. Keep the same `hermoso-profile/v1`
shape, a positive runtime budget, and real profiles that appear in:

```sh
hermes profile list
```

The profile intentionally reuses existing skills:

- `kanban-orchestrator`
- `kanban-worker` (injected by Kanban workers)
- `test-driven-development`
- `systematic-debugging`
- `spike`

Run the non-mutating checks:

```sh
scripts/hermoso-doctor.sh
HERMOSO_PROFILE=/absolute/path/to/profile.yaml scripts/hermoso-doctor.sh
```

The doctor verifies the external directory, profile names, required skill
visibility, Kanban CLI/toolset, and gateway readiness. `--static` performs only
repository checks and is suitable for CI.

Every skill invocation must receive an explicit run ID, canonical repository,
and absolute workspace. Pass explicit context to every `hermoso` command; the
binary validates context against persisted state and rejects mismatches. Never
infer identity from cwd or conversation.

Hermes caches loaded skills for a session. Start a new Hermes session after
changing `skills.external_dirs`, replacing a skill directory, changing the
selected profile, or editing these skill files. Remove the old external
directory entry when replacing this checkout; loading two copies with the same
skill names is unsupported.
