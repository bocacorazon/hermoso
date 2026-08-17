# Tier Model Router Implementation Plan (Go)

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Build a tier-based model router in Go that exposes a single OpenAI-compatible endpoint where the "model" field is an abstract tier name, resolving to concrete models via configurable round-robin.

**Architecture:** A new `internal/router` package in the Hermoso Go codebase, exposed as `hermoso router run`. It's a reverse proxy using `net/http` — no external deps. Config is a YAML file defining tiers, their models, and routing strategy. Repo-level overrides supported.

**Tech Stack:** Go 1.26, `net/http`, `gopkg.in/yaml.v3` (already in go.sum via transitive deps — verify), no framework needed.

---

## Context

The user has:
- 4 local models managed by `local-llm-manager` (ports 8080-8083, one at a time in VRAM)
- An OpenRouter subscription with access to frontier models
- A tiered model strategy (see `docs/tiered-model-strategy.md`) defining 4 tiers: Tier 0 local, Tier 1 cheap frontier, Tier 2 mid frontier, Tier 3 premium

The user wants:
- Abstract tier names that can be used as the "model" in API calls
- Models within a tier are swappable over time (config change, not code change)
- Round-robin within a tier (rotate across models)
- Repo-level overrides (a project can pin specific models for its tiers)
- Global defaults (fallback config)
- **In Go, not Python** — the router lives in the Hermoso codebase

## Design Decisions

1. **Go, stdlib `net/http`.** No web framework. The router is a reverse proxy that reads the request body, rewrites the `model` field, and forwards to the upstream. SSE streaming is handled by flushing the response writer.

2. **New `internal/router` package.** Follows Hermoso's existing `internal/` layout. The router is a self-contained package with its own config loader, tier resolver, and HTTP handler.

3. **`hermoso router` subcommand.** Added to `internal/app/app.go` alongside existing commands. Subcommands: `run`, `start`, `stop`, `restart`, `status`, `init-config`, `list-tiers`.

4. **Config-driven.** All tier definitions live in a YAML file. Swapping a model = edit YAML + restart. Same config format and resolution priority as the Python design:
   - `$LOCAL_LLM_TIERS_PATH` (explicit)
   - `./.local-llm/tiers.yaml` (repo-level)
   - `~/.config/local-llm/tiers.yaml` (global)
   - Bundled default

5. **`${VAR}` env expansion.** Config values like `api_key: ${OPENROUTER_API_KEY}` are expanded from `os.environ` at load time.

6. **In-memory round-robin.** Atomic counter per tier, wraps modulo model count. Resets on restart — acceptable for a long-lived process.

7. **Connection-error fallback.** If a model in a tier returns a connection error, the router automatically tries the next model in the tier (up to len(models) attempts). This handles the "only one local model running at a time" constraint — dead models are skipped transparently.

8. **No new external deps if possible.** Check if `gopkg.in/yaml.v3` is already available transitively. If not, add it to `go.mod`. Everything else is stdlib.

---

## Files to Create/Modify

### New files:
- `internal/router/router.go` — TierRouter struct, config loading, model selection
- `internal/router/handler.go` — HTTP handlers (chat/completions, models, health, embeddings)
- `internal/router/config.go` — config structs, YAML parsing, path resolution, env expansion
- `internal/router/router_test.go` — unit tests for tier resolution and round-robin
- `internal/router/handler_test.go` — HTTP handler tests with mock upstreams
- `config/router/tiers.yaml` — default tier config with all 4 tiers
- `systemd/hermoso-router.service` — systemd unit

### Modified files:
- `internal/app/app.go` — add `router` command to usage string and dispatch
- `go.mod` — add `gopkg.in/yaml.v3` if not already present
- `README.md` — add router section

---

## Task 1: Check yaml.v3 availability and add dependency

**Objective:** Ensure YAML parsing is available.

**Step 1: Check if yaml.v3 is already in go.sum**

Run: `cd /home/marcos/Projects/hermoso && grep yaml.v3 go.sum`
Expected: Either present (transitive dep) or absent.

**Step 2: If absent, add it**

Run: `cd /home/marcos/Projects/hermoso && go get gopkg.in/yaml.v3`
Expected: `go.mod` and `go.sum` updated.

**Step 3: Verify**

Run: `cd /home/marcos/Projects/hermoso && go build ./...`
Expected: Compiles clean.

---

## Task 2: Write config structs and loader

**Objective:** Define the config data structures and the path resolution + env expansion logic.

**Files:**
- Create: `internal/router/config.go`

**Step 1: Write config.go**

```go
package router

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level router configuration.
type Config struct {
	Defaults Defaults         `yaml:"defaults"`
	Tiers    map[string]Tier  `yaml:"tiers"`
}

// Defaults holds router-wide defaults.
type Defaults struct {
	FallbackTier   string `yaml:"fallback_tier"`
	RouterPort     int    `yaml:"router_port"`
	RouterHost     string `yaml:"router_host"`
	RequestTimeout int    `yaml:"request_timeout"`
}

// Tier is a named group of models with a routing strategy.
type Tier struct {
	Strategy string   `yaml:"strategy"` // round-robin, random, first
	Models   []Model  `yaml:"models"`
}

// Model is a single model deployment.
type Model struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`   // local, openrouter
	BaseURL  string `yaml:"base_url"`
	Model    string `yaml:"model"`      // upstream model name
	APIKey   string `yaml:"api_key"`    // "none" or ${VAR}
}

// FindConfigPath resolves the config file location.
// Priority: $LOCAL_LLM_TIERS_PATH > ./.local-llm/tiers.yaml > ~/.config/local-llm/tiers.yaml > bundled
func FindConfigPath() (string, error) {
	if explicit := os.Getenv("LOCAL_LLM_TIERS_PATH"); explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit, nil
		}
	}

	cwd, _ := os.Getwd()
	repoLocal := filepath.Join(cwd, ".local-llm", "tiers.yaml")
	if _, err := os.Stat(repoLocal); err == nil {
		return repoLocal, nil
	}

	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	globalPath := filepath.Join(xdg, "local-llm", "tiers.yaml")
	if _, err := os.Stat(globalPath); err == nil {
		return globalPath, nil
	}

	// Bundled fallback — relative to the binary or repo root
	bundled := filepath.Join(getRepoRoot(), "config", "router", "tiers.yaml")
	if _, err := os.Stat(bundled); err == nil {
		return bundled, nil
	}

	return "", fmt.Errorf("no tiers.yaml found")
}

// LoadConfig reads and parses the config, expanding ${VAR} references.
func LoadConfig() (*Config, error) {
	path, err := FindConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	expanded := expandEnvVars(string(data))
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Defaults.RouterPort == 0 {
		cfg.Defaults.RouterPort = 8090
	}
	if cfg.Defaults.RouterHost == "" {
		cfg.Defaults.RouterHost = "127.0.0.1"
	}
	if cfg.Defaults.RequestTimeout == 0 {
		cfg.Defaults.RequestTimeout = 3600
	}
	return &cfg, nil
}

var envVarRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func expandEnvVars(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		return os.Getenv(varName)
	})
}

func getRepoRoot() string {
	// Walk up from the executable or use a compile-time default
	// In production, the config should be in ~/.config/local-llm/
	// This fallback is for dev runs from the repo.
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for i := 0; i < 10; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			dir = filepath.Dir(dir)
		}
	}
	return "."
}
```

**Step 2: Write a basic test**

```go
// internal/router/config_test.go
package router

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandEnvVars(t *testing.T) {
	os.Setenv("TEST_KEY", "secret123")
	defer os.Unsetenv("TEST_KEY")

	got := expandEnvVars("api_key: ${TEST_KEY}")
	if got != "api_key: secret123" {
		t.Errorf("got %q, want %q", got, "api_key: secret123")
	}
}

func TestFindConfigPath_Explicit(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "tiers.yaml")
	os.WriteFile(path, []byte("tiers: {}"), 0644)

	os.Setenv("LOCAL_LLM_TIERS_PATH", path)
	defer os.Unsetenv("LOCAL_LLM_TIERS_PATH")

	got, err := FindConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Errorf("got %q, want %q", got, path)
	}
}
```

**Step 3: Run tests**

Run: `cd /home/marcos/Projects/hermoso && go test ./internal/router/ -run TestExpandEnvVars -v`
Expected: PASS

Run: `cd /home/marcos/Projects/hermoso && go test ./internal/router/ -run TestFindConfigPath -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/router/config.go internal/router/config_test.go go.mod go.sum
git commit -m "feat(router): add config structs, YAML loading, env expansion"
```

---

## Task 3: Write the TierRouter — tier resolution and model selection

**Objective:** The core routing logic: resolve a model field to a tier, select a model via strategy, with connection-error fallback.

**Files:**
- Create: `internal/router/router.go`

**Step 1: Write router.go**

```go
package router

import (
	"fmt"
	"math/rand"
	"sync/atomic"
)

// TierRouter holds config and round-robin state.
type TierRouter struct {
	config  *Config
	counters map[string]*atomic.Uint64 // tier_name -> counter
}

// NewTierRouter creates a router from the given config.
func NewTierRouter(cfg *Config) *TierRouter {
	counters := make(map[string]*atomic.Uint64)
	for name := range cfg.Tiers {
		counters[name] = new(atomic.Uint64)
	}
	return &TierRouter{config: cfg, counters: counters}
}

// ResolveTier maps the model field to a tier name and tier config.
// If the model field is not a known tier, uses the fallback tier.
func (r *TierRouter) ResolveTier(modelField string) (string, *Tier) {
	if tier, ok := r.config.Tiers[modelField]; ok {
		return modelField, &tier
	}
	fallback := r.config.Defaults.FallbackTier
	if fallback == "" {
		fallback = "tier-0-local"
	}
	if tier, ok := r.config.Tiers[fallback]; ok {
		return fallback, &tier
	}
	// Last resort: first tier
	for name, tier := range r.config.Tiers {
		return name, &tier
	}
	return "", nil
}

// SelectModel picks a model from the tier using the configured strategy.
// startIndex is used for fallback — on connection error, caller passes
// (selectedIndex + 1) % len to try the next model.
func (r *TierRouter) SelectModel(tierName string, tier *Tier, startIndex int) (*Model, error) {
	if len(tier.Models) == 0 {
		return nil, fmt.Errorf("no models in tier %q", tierName)
	}
	idx := startIndex % len(tier.Models)

	switch tier.Strategy {
	case "random":
		idx = rand.Intn(len(tier.Models))
	case "first":
		idx = 0
	default: // round-robin
		if startIndex < 0 {
			counter := r.counters[tierName]
			if counter != nil {
				idx = int(counter.Add(1)-1) % len(tier.Models)
			}
		}
	}

	model := &tier.Models[idx]
	return model, nil
}

// ModelOrder returns the order in which models should be tried for fallback.
// For round-robin: starts at the current counter, cycles through all.
// For first: just the first model.
// For random: the random pick, then all others.
func (r *TierRouter) ModelOrder(tierName string, tier *Tier) []int {
	n := len(tier.Models)
	if n == 0 {
		return nil
	}

	var start int
	switch tier.Strategy {
	case "first":
		return []int{0}
	case "random":
		start = rand.Intn(n)
	default: // round-robin
		counter := r.counters[tierName]
		if counter != nil {
			start = int(counter.Add(1)-1) % n
		}
	}

	order := make([]int, n)
	for i := 0; i < n; i++ {
		order[i] = (start + i) % n
	}
	return order
}

// ListTiers returns a summary of all tiers and their models.
func (r *TierRouter) ListTiers() map[string][]string {
	result := make(map[string][]string)
	for name, tier := range r.config.Tiers {
		models := make([]string, len(tier.Models))
		for i, m := range tier.Models {
			models[i] = fmt.Sprintf("%s (%s)", m.Name, m.Model)
		}
		result[name] = models
	}
	return result
}
```

**Step 2: Write tests for round-robin and resolution**

```go
// internal/router/router_test.go
package router

import (
	"testing"
)

func testConfig() *Config {
	return &Config{
		Defaults: Defaults{FallbackTier: "tier-0-local"},
		Tiers: map[string]Tier{
			"tier-0-local": {
				Strategy: "round-robin",
				Models: []Model{
					{Name: "a", Model: "model-a"},
					{Name: "b", Model: "model-b"},
					{Name: "c", Model: "model-c"},
				},
			},
			"tier-1-cheap": {
				Strategy: "first",
				Models: []Model{
					{Name: "x", Model: "model-x"},
				},
			},
		},
	}
}

func TestResolveTier_Direct(t *testing.T) {
	r := NewTierRouter(testConfig())
	name, tier := r.ResolveTier("tier-0-local")
	if name != "tier-0-local" || tier == nil {
		t.Fatalf("expected tier-0-local, got %s", name)
	}
}

func TestResolveTier_Fallback(t *testing.T) {
	r := NewTierRouter(testConfig())
	name, _ := r.ResolveTier("unknown-model")
	if name != "tier-0-local" {
		t.Errorf("expected fallback to tier-0-local, got %s", name)
	}
}

func TestModelOrder_RoundRobin(t *testing.T) {
	r := NewTierRouter(testConfig())
	tier := r.config.Tiers["tier-0-local"]

	// First call should start at 0 (counter starts at 0, Add returns 1, idx=0)
	order1 := r.ModelOrder("tier-0-local", &tier)
	if len(order1) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(order1))
	}
	// Should be a rotation: 0,1,2 or 1,2,0 or 2,0,1
	if order1[0] == order1[1] || order1[1] == order1[2] || order1[0] == order1[2] {
		t.Errorf("expected all unique indices, got %v", order1)
	}

	// Second call should start at the next index
	order2 := r.ModelOrder("tier-0-local", &tier)
	if order2[0] == order1[0] {
		t.Errorf("expected different start on second call, both started at %d", order1[0])
	}
}

func TestModelOrder_First(t *testing.T) {
	r := NewTierRouter(testConfig())
	tier := r.config.Tiers["tier-1-cheap"]
	order := r.ModelOrder("tier-1-cheap", &tier)
	if len(order) != 1 || order[0] != 0 {
		t.Errorf("expected [0], got %v", order)
	}
}
```

**Step 3: Run tests**

Run: `cd /home/marcos/Projects/hermoso && go test ./internal/router/ -v`
Expected: All PASS

**Step 4: Commit**

```bash
git add internal/router/router.go internal/router/router_test.go
git commit -m "feat(router): add TierRouter with round-robin and fallback"
```

---

## Task 4: Write HTTP handlers — chat/completions, models, health

**Objective:** The HTTP layer that receives OpenAI-format requests, rewrites the model field, proxies to upstream, and handles streaming + connection-error fallback.

**Files:**
- Create: `internal/router/handler.go`

**Step 1: Write handler.go**

Key components:

1. **`Handler` struct** — holds a `*TierRouter` and an `*http.Client` with configurable timeout.

2. **`chatCompletions(w, r)`** — the main endpoint:
   - Read request body, parse JSON to get `model` field and `stream` field
   - Resolve tier, get model order for fallback
   - For each model in order:
     - Rewrite `model` field to the upstream model name
     - Build upstream URL: `base_url + "/chat/completions"`
     - Set auth header (Bearer api_key, or omit if "none")
     - Add OpenRouter headers if provider is openrouter
     - If streaming: proxy SSE chunks with `http.Flusher`, return on first successful connection
     - If non-streaming: proxy response, set `X-Tier-*` headers, return
     - On connection error: log, try next model in order
   - If all models fail: return 503

3. **`listModels(w, r)`** — returns tiers as OpenAI-format model list (one entry per tier).

4. **`health(w, r)`** — returns router status.

5. **`embeddings(w, r)`** — same proxy logic for embeddings.

6. **`Routes()`** — returns an `*http.ServeMux` with all routes registered.

**Streaming implementation:**
```go
// For streaming, we pipe the upstream response body to the client
// using io.Copy with manual flushing.
func (h *Handler) proxyStream(w http.ResponseWriter, resp *http.Response) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
	return nil
}
```

**Fallback implementation:**
```go
func (h *Handler) chatCompletions(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var payload map[string]any
	json.Unmarshal(body, &payload)

	modelField, _ := payload["model"].(string)
	tierName, tier := h.router.ResolveTier(modelField)
	if tier == nil {
		http.Error(w, "no tier configured", http.StatusInternalServerError)
		return
	}

	order := h.router.ModelOrder(tierName, tier)
	for _, idx := range order {
		model := &tier.Models[idx]
		payload["model"] = model.Model

		// Build upstream request
		upstreamURL := strings.TrimRight(model.BaseURL, "/") + "/chat/completions"
		req, _ := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(mustMarshal(payload)))
		req.Header.Set("Content-Type", "application/json")
		if model.APIKey != "" && model.APIKey != "none" {
			req.Header.Set("Authorization", "Bearer "+model.APIKey)
		}
		if model.Provider == "openrouter" {
			req.Header.Set("HTTP-Referer", "https://github.com/bocacorazon/hermoso")
			req.Header.Set("X-Title", "hermoso-tier-router")
		}

		resp, err := h.client.Do(req)
		if err != nil {
			// Connection error — try next model
			log.Printf("tier %s model %s: connection error: %v, trying next", tierName, model.Name, err)
			continue
		}

		// Success — proxy the response
		isStream, _ := payload["stream"].(bool)
		if isStream {
			defer resp.Body.Close()
			h.proxyStream(w, resp)
		} else {
			defer resp.Body.Close()
			w.Header().Set("X-Tier-Model", model.Name)
			w.Header().Set("X-Tier-Provider", model.Provider)
			w.Header().Set("X-Tier-Upstream-Model", model.Model)
			w.Header().Set("Content-Type", "application/json")
			io.Copy(w, resp.Body)
		}
		return
	}

	// All models failed
	http.Error(w, fmt.Sprintf("all models in tier %q failed", tierName), http.StatusServiceUnavailable)
}
```

**Step 2: Write handler tests with mock upstreams**

```go
// internal/router/handler_test.go
package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Mock upstream that returns a fixed chat completion
func mockUpstream(t *testing.T, status int, response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(response))
	}))
}

func TestChatCompletions_RoutesToTier(t *testing.T) {
	upstream := mockUpstream(t, 200, `{"choices":[{"message":{"content":"OK"}}]}`)
	defer upstream.Close()

	cfg := &Config{
		Defaults: Defaults{FallbackTier: "tier-0-local"},
		Tiers: map[string]Tier{
			"tier-0-local": {
				Strategy: "first",
				Models: []Model{
					{Name: "test", Provider: "local", BaseURL: upstream.URL, Model: "test-model", APIKey: "none"},
				},
			},
		},
	}
	router := NewTierRouter(cfg)
	handler := NewHandler(router, 30)

	reqBody := `{"model":"tier-0-local","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.chatCompletions(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "OK") {
		t.Errorf("expected response to contain 'OK', got %s", w.Body.String())
	}
	if w.Header().Get("X-Tier-Model") != "test" {
		t.Errorf("expected X-Tier-Model=test, got %s", w.Header().Get("X-Tier-Model"))
	}
}

func TestChatCompletions_FallbackOnConnectionError(t *testing.T) {
	// First model points to a dead port, second to a live mock
	live := mockUpstream(t, 200, `{"choices":[{"message":{"content":"OK"}}]}`)
	defer live.Close()

	cfg := &Config{
		Defaults: Defaults{FallbackTier: "tier-0-local"},
		Tiers: map[string]Tier{
			"tier-0-local": {
				Strategy: "first",
				Models: []Model{
					{Name: "dead", Provider: "local", BaseURL: "http://127.0.0.1:1", Model: "dead-model", APIKey: "none"},
					{Name: "live", Provider: "local", BaseURL: live.URL, Model: "live-model", APIKey: "none"},
				},
			},
		},
	}
	router := NewTierRouter(cfg)
	handler := NewHandler(router, 5) // short timeout for fast test

	reqBody := `{"model":"tier-0-local","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.chatCompletions(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 after fallback, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Tier-Model") != "live" {
		t.Errorf("expected fallback to 'live' model, got X-Tier-Model=%s", w.Header().Get("X-Tier-Model"))
	}
}
```

**Step 3: Run tests**

Run: `cd /home/marcos/Projects/hermoso && go test ./internal/router/ -v`
Expected: All PASS

**Step 4: Commit**

```bash
git add internal/router/handler.go internal/router/handler_test.go
git commit -m "feat(router): add HTTP handlers with streaming and fallback"
```

---

## Task 5: Write the default tiers.yaml config

**Objective:** Ready-to-use config with all 4 tiers.

**Files:**
- Create: `config/router/tiers.yaml`

```yaml
defaults:
  fallback_tier: tier-0-local
  router_port: 8090
  router_host: 127.0.0.1
  request_timeout: 3600

tiers:
  tier-0-local:
    strategy: round-robin
    models:
      - name: qwen3-coder
        provider: local
        base_url: http://127.0.0.1:8083/v1
        model: qwen3-coder-30b
        api_key: none
      - name: qwen-64k
        provider: local
        base_url: http://127.0.0.1:8080/v1
        model: qwen-35b-a3b-64k
        api_key: none
      - name: qwen-27b
        provider: local
        base_url: http://127.0.0.1:8081/v1
        model: qwen2.5-27b
        api_key: none
      - name: gemma-31b
        provider: local
        base_url: http://127.0.0.1:8082/v1
        model: gemma-4-31b-it
        api_key: none
      - name: qwen-3.8-27b
        provider: local
        base_url: http://127.0.0.1:8084/v1
        model: qwen-3.8-27b
        api_key: none
        # batch-only thinking model: correctness-optimized, not for interactive
        # routing. Assign explicitly to E2 (judge) and correctness-critical C2.

  tier-1-cheap:
    strategy: round-robin
    models:
      - name: claude-haiku
        provider: openrouter
        base_url: https://openrouter.ai/api/v1
        model: anthropic/claude-haiku-4
        api_key: ${OPENROUTER_API_KEY}
      - name: gpt-4o-mini
        provider: openrouter
        base_url: https://openrouter.ai/api/v1
        model: openai/gpt-4o-mini
        api_key: ${OPENROUTER_API_KEY}
      - name: gemini-flash
        provider: openrouter
        base_url: https://openrouter.ai/api/v1
        model: google/gemini-2.0-flash-001
        api_key: ${OPENROUTER_API_KEY}
      - name: deepseek-chat
        provider: openrouter
        base_url: https://openrouter.ai/api/v1
        model: deepseek/deepseek-chat
        api_key: ${OPENROUTER_API_KEY}

  tier-2-mid:
    strategy: round-robin
    models:
      - name: claude-sonnet
        provider: openrouter
        base_url: https://openrouter.ai/api/v1
        model: anthropic/claude-sonnet-4
        api_key: ${OPENROUTER_API_KEY}
      - name: deepseek-v4
        provider: openrouter
        base_url: https://openrouter.ai/api/v1
        model: deepseek/deepseek-v4
        api_key: ${OPENROUTER_API_KEY}
      - name: gpt-4.1
        provider: openrouter
        base_url: https://openrouter.ai/api/v1
        model: openai/gpt-4.1
        api_key: ${OPENROUTER_API_KEY}

  tier-3-premium:
    strategy: round-robin
    models:
      - name: claude-opus
        provider: openrouter
        base_url: https://openrouter.ai/api/v1
        model: anthropic/claude-opus-4
        api_key: ${OPENROUTER_API_KEY}
      - name: o3
        provider: openrouter
        base_url: https://openrouter.ai/api/v1
        model: openai/o3
        api_key: ${OPENROUTER_API_KEY}
```

---

## Task 6: Add `hermoso router` subcommand to app.go

**Objective:** Integrate router management into the existing CLI.

**Files:**
- Modify: `internal/app/app.go`

Add to the usage string:
```
  hermoso router <run|start|stop|restart|status|init-config|list-tiers> ...
```

Add to the command list:
```
  router     Start or manage the tier model router
```

Add to the dispatch switch:
```go
case "router":
	return cmdRouter(ctx, deps, args[1:])
```

Implement `cmdRouter`:
```go
func cmdRouter(ctx context.Context, deps Dependencies, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(deps.Stderr, "usage: hermoso router <run|start|stop|restart|status|init-config|list-tiers>")
		return ExitUsage
	}
	switch args[0] {
	case "run":
		return routerRun(ctx, deps, args[1:])
	case "start":
		return routerSystemd(ctx, deps, "start")
	case "stop":
		return routerSystemd(ctx, deps, "stop")
	case "restart":
		return routerSystemd(ctx, deps, "restart")
	case "status":
		return routerSystemd(ctx, deps, "status")
	case "init-config":
		return routerInitConfig(ctx, deps)
	case "list-tiers":
		return routerListTiers(ctx, deps)
	default:
		fmt.Fprintf(deps.Stderr, "unknown router subcommand: %s\n", args[0])
		return ExitUsage
	}
}
```

**`routerRun`** — loads config, creates the handler, starts the HTTP server:
```go
func routerRun(ctx context.Context, deps Dependencies, args []string) int {
	cfg, err := router.LoadConfig()
	if err != nil {
		fmt.Fprintf(deps.Stderr, "error loading config: %v\n", err)
		return ExitFailure
	}
	r := router.NewTierRouter(cfg)
	h := router.NewHandler(r, cfg.Defaults.RequestTimeout)
	mux := h.Routes()

	addr := fmt.Sprintf("%s:%d", cfg.Defaults.RouterHost, cfg.Defaults.RouterPort)
	fmt.Fprintf(deps.Stdout, "Tier router starting on %s\n", addr)
	fmt.Fprintf(deps.Stdout, "Tiers: %v\n", r.ListTiers())

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(deps.Stderr, "server error: %v\n", err)
		return ExitFailure
	}
	return ExitOK
}
```

**`routerSystemd`** — shells out to `systemctl --user`:
```go
func routerSystemd(ctx context.Context, deps Dependencies, action string) int {
	cmd := exec.Command("systemctl", "--user", action, "hermoso-router.service")
	cmd.Stdout = deps.Stdout
	cmd.Stderr = deps.Stderr
	if err := cmd.Run(); err != nil {
		return ExitFailure
	}
	return ExitOK
}
```

**`routerInitConfig`** — copies bundled tiers.yaml to `~/.config/local-llm/tiers.yaml`:
```go
func routerInitConfig(ctx context.Context, deps Dependencies) int {
	// Check if already exists
	path, err := router.FindConfigPath()
	if err == nil {
		fmt.Fprintf(deps.Stdout, "Config already exists: %s\n", path)
		return ExitOK
	}
	// Copy bundled default
	home, _ := os.UserHomeDir()
	dest := filepath.Join(home, ".config", "local-llm", "tiers.yaml")
	os.MkdirAll(filepath.Dir(dest), 0755)
	// ... copy bundled config/router/tiers.yaml to dest
	fmt.Fprintf(deps.Stdout, "Created: %s\n", dest)
	return ExitOK
}
```

**`routerListTiers`** — loads config and prints a table:
```go
func routerListTiers(ctx context.Context, deps Dependencies) int {
	cfg, err := router.LoadConfig()
	if err != nil {
		fmt.Fprintf(deps.Stderr, "error: %v\n", err)
		return ExitFailure
	}
	r := router.NewTierRouter(cfg)
	for name, models := range r.ListTiers() {
		fmt.Fprintf(deps.Stdout, "%s (%s):\n", name, cfg.Tiers[name].Strategy)
		for _, m := range models {
			fmt.Fprintf(deps.Stdout, "  - %s\n", m)
		}
	}
	return ExitOK
}
```

**Step 2: Verify build**

Run: `cd /home/marcos/Projects/hermoso && go build ./cmd/hermoso`
Expected: Compiles clean

**Step 3: Test list-tiers**

Run: `cd /home/marcos/Projects/hermoso && ./hermoso router list-tiers`
Expected: Prints 4 tiers with their models

**Step 4: Commit**

```bash
git add internal/app/app.go internal/router/
git commit -m "feat(router): add 'hermoso router' subcommand with run/start/stop/list-tiers"
```

---

## Task 7: Write systemd service

**Objective:** Auto-start and restart the router.

**Files:**
- Create: `systemd/hermoso-router.service`

```ini
[Unit]
Description=Hermoso Tier Model Router
After=default.target

[Service]
Type=simple
Environment=LOCAL_LLM_TIERS_PATH=%h/.config/local-llm/tiers.yaml
Environment=ROUTER_HOST=127.0.0.1
Environment=ROUTER_PORT=8090
ExecStart=%h/.local/bin/hermoso router run
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=default.target
```

---

## Task 8: Update README

**Objective:** Document the tier router.

**Files:**
- Modify: `README.md`

Add a section covering:
- Quick start (init-config, start, point Hermes at it)
- Tier table (4 tiers, purpose, example models)
- How to swap models (edit YAML, restart)
- Repo-level overrides (`.local-llm/tiers.yaml`)
- Round-robin explanation
- Connection-error fallback behavior

---

## Task 9: End-to-end test

**Step 1: Build and run**

Run: `cd /home/marcos/Projects/hermoso && go build -o hermoso ./cmd/hermoso && ./hermoso router run &`
Expected: "Tier router starting on 127.0.0.1:8090"

**Step 2: Health check**

Run: `curl -s http://127.0.0.1:8090/health`
Expected: `{"status":"ok","tiers":4,...}`

**Step 3: List models**

Run: `curl -s http://127.0.0.1:8090/v1/models`
Expected: 4 tier entries

**Step 4: Test chat with local model**

Prerequisite: `local-llm start qwen3coder`

Run:
```bash
curl -s http://127.0.0.1:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"tier-0-local","messages":[{"role":"user","content":"Reply with OK only."}],"max_tokens":16}'
```
Expected: Valid completion, `X-Tier-Model` header set

**Step 5: Test fallback**

With only qwen3coder running, the router should skip qwen-64k, qwen-27b, and gemma-31b (connection refused) and land on qwen3coder.

**Step 6: Test streaming**

Run with `"stream":true` — expect SSE chunks ending with `[DONE]`.

**Step 7: Test unknown model fallback**

Run with `"model":"unknown"` — should fall back to tier-0-local.

**Step 8: Commit**

```bash
git add -A
git commit -m "feat(router): complete tier model router with tests, config, systemd"
```

---

## Risks and Open Questions

1. **yaml.v3 dependency.** If not already in go.sum, we add one new dep. This is the most widely used YAML library in Go and is stable.

2. **Connection-error fallback for local models.** Implemented in Task 4 — the router tries each model in the tier on connection failure. This handles the "only one local model running" constraint. The fallback has a short connection timeout (5s default, configurable) so dead models are skipped quickly.

3. **Streaming correctness.** Go's `net/http` supports `http.Flusher` for SSE. The handler flushes after each read chunk. Need to verify no buffering issues with OpenRouter's SSE format.

4. **Config hot-reload.** Not implemented. Config is loaded at startup. To pick up changes, restart the router (`hermoso router restart`). Could add SIGHUP-based reload later.

5. **No auth on router.** Listens on 127.0.0.1 only. Add API key middleware if network exposure is needed later.

6. **OpenRouter model names.** The specific models in tiers.yaml are recommendations. User should verify availability on their OpenRouter plan. Swapping = config change.

7. **Integration with Hermoso lifecycle.** The router is a standalone service — Hermoso workers call it via HTTP. No code changes to Hermoso's workflow/domain packages. The router is infrastructure, not part of the lifecycle.
