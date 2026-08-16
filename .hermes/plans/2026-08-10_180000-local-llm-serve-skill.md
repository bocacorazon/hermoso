# Local LLM Bring-Online Skill — Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Create a Hermes skill that starts a local model via `local-llm-manager`, verifies it is healthy, and switches Hermes to use it as the active provider — all in one command.

**Architecture:** A single SKILL.md skill file with no supporting scripts. The skill wraps `local-llm` CLI commands (start, smoke-test, status) and `hermes config set` to switch the active model/provider. It handles the VRAM constraint (only one model at a time) by stopping any running instance first.

**Tech Stack:** Bash (local-llm CLI), Hermes config CLI, YAML config

**Workspace:** `/home/marcos/Projects/hermoso` (skills live at `skills/`)

---

## Context

### What exists

- `local-llm-manager` at `/home/marcos/Projects/local-llm-manager` — systemd-backed LLM runtime manager
- `local-llm` CLI installed at `~/.local/bin/local-llm` — commands: start, stop, restart, status, logs, smoke-test, list-configs
- Three configured instances: `qwen64k` (port 8080), `qwen27b` (port 8081), `gemma31b` (port 8082)
- `rocm-distrobox-model-setup` skill already exists for *adding new models* — this skill is for *bringing an existing model online*
- Hermes config at `~/.hermes/config.yaml` with `model`, `provider`, `base_url`, `api_key`, `context_length` fields
- Current Hermes config points at `gemma-4-31b` on port 8082

### What the skill does

1. Lists available model instances (from `local-llm list-configs`)
2. If another model is running, stops it (VRAM constraint: one model at a time)
3. Starts the requested instance
4. Waits for the model to load into VRAM (polls `/v1/models` endpoint)
5. Runs smoke-test to verify chat completions work
6. Updates Hermes config to point at the new model
7. Reports success with the model name, port, and Hermes config

### Instance → Hermes config mapping

Each instance has an env file at `~/.config/local-llm/instances/<name>.env` with:
- `MODEL_ALIAS` — the model name for the API (e.g., `gemma-4-31b`)
- `PORT` — the port (e.g., `8082`)
- `CTX_SIZE` — context length (e.g., `131072`)

The skill reads these from the env file to construct the Hermes config values.

### VRAM constraint

The R9700 has 32 GB VRAM. Only one model fits at a time. The skill must stop any running instance before starting a new one.

---

## Files

### Create
- `skills/local-llm-serve/SKILL.md` — the skill file

### Modify
- None (skill is self-contained)

---

## Tasks

### Task 1: Create the SKILL.md skill file

**Objective:** Write the complete skill with frontmatter, usage, and step-by-step workflow.

**Files:**
- Create: `skills/local-llm-serve/SKILL.md`

**Step 1: Write the skill file**

```markdown
---
name: local-llm-serve
description: "Start a local LLM via local-llm-manager, verify it, and switch Hermes to use it. Handles VRAM constraint (one model at a time)."
version: 1.0.0
author: Hermes Agent
license: MIT
platforms: [linux]
metadata:
  hermes:
    tags: [local-llm, llama.cpp, distrobox, vulkan, model-serving, hermes-config]
    related_skills: [rocm-distrobox-model-setup, llama-cpp]
---

# Local LLM Serve

Use this skill when the user wants to start a local model and switch Hermes
to use it as the active provider.

## When to use

- "Start gemma31b" / "bring online qwen27b" / "switch to qwen64k"
- "What local models are available?"
- "Restart the current model"
- "Stop the local model"

## Environment facts

- `local-llm` CLI at `~/.local/bin/local-llm`
- Instance configs at `~/.config/local-llm/instances/<name>.env`
- GPU: AMD Radeon AI PRO R9700, 32 GB VRAM — only one model at a time
- Distrobox container: `r9700-llama-vulkan-radv` (Vulkan/RADV backend)
- Hermes config: `~/.hermes/config.yaml`
- Hermes config CLI: `hermes config set <key> <value>`

## Available instances

| Instance  | Model              | Port  | VRAM est. |
|-----------|--------------------|-------|-----------|
| qwen64k   | Qwen3.6 64K        | 8080  | ~22 GB    |
| qwen27b   | Qwen3.6 27B Q4_K_M | 8081  | ~17 GB    |
| gemma31b  | Gemma 4 31B Q4_K_M | 8082  | ~19 GB    |

## Workflow: Start a model

### 1. List available instances

```bash
local-llm list-configs
```

### 2. Check what is currently running

```bash
local-llm status <instance>   # for each, or check systemctl --user list-units 'local-llm@*'
```

Quick check for any running instance:

```bash
systemctl --user list-units --type=service --all 'local-llm@*' --no-legend --no-pager | grep active
```

### 3. Stop any running instance (VRAM constraint)

If a different instance is running, stop it first:

```bash
local-llm stop <other-instance>
```

Wait a few seconds for VRAM to free:

```bash
sleep 3
```

### 4. Start the requested instance

```bash
local-llm start <instance>
```

### 5. Wait for model to load

Models take 10-30 seconds to load into VRAM. Poll the health endpoint:

```bash
# Read PORT from instance env file
source ~/.config/local-llm/instances/<instance>.env

# Poll until ready (max 60 seconds)
for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${PORT}/v1/models" >/dev/null 2>&1; then
    echo "Model ready on port ${PORT}"
    break
  fi
  sleep 2
done
```

If the endpoint is not up after 60 seconds, check logs:

```bash
local-llm logs <instance> -n 30
```

### 6. Smoke-test

```bash
local-llm smoke-test <instance>
```

Verify both the `/v1/models` and `/v1/chat/completions` endpoints respond.

### 7. Switch Hermes to the new model

Read the instance config for `MODEL_ALIAS`, `PORT`, and `CTX_SIZE`:

```bash
source ~/.config/local-llm/instances/<instance>.env
```

Update Hermes config:

```bash
hermes config set model "${MODEL_ALIAS}"
hermes config set provider custom
hermes config set base_url "http://127.0.0.1:${PORT}/v1"
hermes config set api_key none
hermes config set context_length "${CTX_SIZE}"
```

### 8. Report success

Report to the user:
- Model name: `<MODEL_ALIAS>`
- Endpoint: `http://127.0.0.1:<PORT>/v1`
- Context length: `<CTX_SIZE>`
- Hermes has been switched to use this model

## Workflow: Stop the current model

```bash
# Find running instance
systemctl --user list-units --type=service --all 'local-llm@*' --no-legend --no-pager | grep active

# Stop it
local-llm stop <instance>
```

## Workflow: Restart the current model

```bash
local-llm restart <instance>
# Then poll for readiness as in step 5 above
```

## Workflow: Check status

```bash
local-llm status <instance>
local-llm logs <instance> -n 20
```

## Pitfalls

- **Only one model at a time**: 32 GB VRAM fits one model + KV cache. Always stop the current model before starting a new one.
- **Distrobox auto-start**: The distrobox container auto-starts if stopped, but this adds ~5 seconds to the first start.
- **HOST=0.0.0.0**: Instances listen on 0.0.0.0 but smoke-test and Hermes connect to 127.0.0.1.
- **Context length**: Each instance has its own CTX_SIZE. Make sure to update Hermes `context_length` to match, not hardcode it.
- **Model alias must match**: The `MODEL_ALIAS` in the instance env must match what the `/v1/models` endpoint reports. If it doesn't, Hermes chat requests will fail with "model not found".
- **hermes config set requires restart**: After changing Hermes config, the user should start a new Hermes session for the change to take effect.
```

**Step 2: Verify the skill file exists and is valid**

```bash
cat skills/local-llm-serve/SKILL.md
```

**Step 3: Run skills test (if applicable)**

```bash
cd /home/marcos/Projects/hermoso && go test ./skills/ -v
```

Expected: PASS (the skills test checks frontmatter and cross-references; this skill should pass)

**Step 4: Commit**

```bash
git add skills/local-llm-serve/SKILL.md
git commit -m "feat: add local-llm-serve skill for starting models and switching Hermes"
```

---

### Task 2: Install the skill into Hermes

**Objective:** Copy the skill into `~/.hermes/skills/` so Hermes can load it.

**Files:**
- Create: `~/.hermes/skills/mlops/local-llm-serve/SKILL.md`

**Step 1: Create the directory and copy**

```bash
mkdir -p ~/.hermes/skills/mlops/local-llm-serve
cp skills/local-llm-serve/SKILL.md ~/.hermes/skills/mlops/local-llm-serve/SKILL.md
```

**Step 2: Verify Hermes can see it**

```bash
hermes skills list 2>&1 | grep local-llm-serve
```

Expected: the skill appears in the list

**Step 3: Commit (the Hermoso repo copy)**

Already committed in Task 1. No additional commit needed.

---

## Verification

### Manual test

1. Ask Hermes: "start gemma31b"
2. Hermes should load the skill, run `local-llm start gemma31b`, poll for readiness, smoke-test, and update config
3. Verify: `hermes config show` shows `gemma-4-31b` on port 8082
4. Start a new Hermes session and chat — it should use the local model

### Automated test

The Hermoso `skills/` test suite checks frontmatter validity. Run:

```bash
cd /home/marcos/Projects/hermoso && go test ./skills/ -v -run TestSkillFrontmatter
```

---

## Risks and Tradeoffs

### No scripts
The skill is pure SKILL.md — no supporting scripts. All commands are inline bash that Hermes executes via the terminal tool. This keeps the skill portable and avoids path issues.

### Instance config parsing
The skill sources the instance env file (`source ~/.config/local-llm/instances/<name>.env`) to read `MODEL_ALIAS`, `PORT`, and `CTX_SIZE`. This is safe because the files are generated by `local-llm config init` and contain only `KEY=value` pairs.

### Hermes config persistence
`hermes config set` writes to `~/.hermes/config.yaml`. The change persists across sessions. The user should start a new session for the change to take effect — the skill should note this.

### VRAM safety
The skill always checks for running instances and stops them first. If the user tries to start a model without stopping the current one, the new model will fail to load (OOM) and the systemd unit will restart-loop.

### Distrobox dependency
The skill depends on the distrobox container being functional. If the container is broken, `local-llm start` will fail. The skill should check logs if startup fails.
