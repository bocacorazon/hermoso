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

| Instance    | Model                       | Port  | VRAM est. |
|-------------|-----------------------------|-------|-----------|
| qwen64k     | Qwen3.6 64K                 | 8080  | ~22 GB    |
| qwen27b     | Qwen3.6 27B Q4_K_M          | 8081  | ~17 GB    |
| gemma31b    | Gemma 4 31B Q4_K_M          | 8082  | ~19 GB    |
| qwen3coder  | Qwen3-Coder-30B-A3B UD-Q4_K_XL | 8083 | ~18 GB |

## Workflow: Start a model

### 1. List available instances

```bash
local-llm list-configs
```

### 2. Check what is currently running

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
- The user should start a new Hermes session for the change to take effect

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
