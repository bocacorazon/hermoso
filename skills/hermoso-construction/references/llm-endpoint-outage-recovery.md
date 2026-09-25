# Local-LLM Endpoint Outage During Construction — Diagnosis & Recovery

Incident class: the model server backing kanban workers dies or restarts
mid-construction. Workers zombify, kanban shows stale `running` claims, and
the user may report the server "idle with no model loaded". Seen live:
kernel OOM killer (via systemd-oomd) killed llama-server (exit 137) while
two concurrent workers were mid-turn on a 29 GB host.

## 1. Detect

- `hermes kanban runs <task-id> --json` reports `status: running`, but
  `ps -p <worker_pid>` returns nothing → stale claim.
- Task logs (`~/.hermes/kanban/logs/<task-id>.log`) stop timestamping at
  the restart moment (both workers ended at the same wall-clock time —
  that coincidence is itself a tell).
- `curl http://127.0.0.1:<port>/health` may still say ok (systemd
  auto-restarted the service) — the real tell is `/metrics` token
  counters reset to 0.
- `local-llm status <instance>` shows a recent Active-since time the
  orchestrator did not cause.

## 2. Diagnose root cause

```sh
journalctl --user -u local-llm@<instance> --since "<window>" --no-pager \
  | grep -E "Start|Stop|Fail|kill|oom|signal|exit"
journalctl -k --since "<window>" | grep -iE "oom|out of memory|Killed process"
free -h
ps -o pid,rss --no-headers -C llama-server   # RSS in KB
```

Exit 137 = SIGKILL. `Out of memory: Killed process <pid> (llama-server)`
in the kernel log names the OOM killer; the user-scope journal shows
`systemd-oomd invoked oom-killer`. Container scopes (libpod) get
`oom_score_adj=200` (kill-me-first), so the LLM server is the preferred
victim under host memory pressure. The scope's journal line also reports
its memory peak ("17.6G memory peak" style) — useful evidence.

Distinguish from a user-initiated restart by ASKING before assuming —
in the live incident the user confirmed "i did not restart it", which
justified the journalctl dig.

## 3. Recover workers

Uncommitted worktree changes survive the crash — do not reset anything:

```sh
hermes kanban reclaim <task-id>        # resets stale claim to ready
hermes kanban dispatch --max N --json  # respawns; idempotency keys dedupe
```

Verify respawn: `hermes kanban runs <id> --json` shows a fresh run with a
live `worker_pid`; `ps -p <pid> -o args` shows the expected
`-m <model> --provider <provider>` flags; local `/metrics` token counters
climb again.

Note: a task whose card was already BOUND but never spawned (lost race /
OOM window) will NOT reappear in `construction ready` (bound cards are
skipped) — just dispatch it directly.

## 4. Prevent (memory sizing)

Before dispatching 2+ concurrent workers on a small-RAM host:

- `free -h` — leave real headroom; swap in use is not headroom.
- llama-server RSS grows toward weights + KV cache as session contexts
  warm up (observed 9 GB cold → 18.4 GB before the kill).
- Size `CTX_SIZE` in `~/.config/local-llm/instances/<instance>.env` to the
  workload. Hermoso construction cards run ~20-60K tokens — 131072 was
  the OOM trigger with two workers on 29 GB; the user chose
  "2 workers at 64k" as the balance.
- CTX_SIZE takes effect at next server restart. Apply the env edit, then
  bounce the server in a gap between worker runs — never mid-turn.
- Update client-side context metadata to match, so worker sessions
  compact before the new window instead of overflowing it:
  `model.aliases.<name>.context_length` and the matching
  `custom_providers.<name>.models.<id>.context_length` in
  `~/.hermes/config.yaml`.

## 5. Verification-environment prep

Verification contracts may pin an absolute interpreter path in every
judgment command (e.g. `/home/marcos/.venvs/<name>/bin/python`). That
venv must exist with the project's full dependency set BEFORE
`verification run` or every judgment fails. Rebuild during construction:

```sh
uv venv ~/.venvs/<name>
uv pip install --python ~/.venvs/<name>/bin/python \
  -r <repo>/requirements.txt pytest
```

Check importability of the key deps first — a venv without pip and
missing pydantic fails with confusing per-test collection errors, not a
clear "venv broken" message.
