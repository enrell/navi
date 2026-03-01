# Agents

This document is the authoritative technical reference for Navi's agent system.

## Design Philosophy

**Agents are data, not code.** A Navi agent is fully defined by two files:

| File | Purpose |
|------|---------|
| `config.toml` | Identity, LLM, capabilities, isolation backend |
| `AGENT.md` | System prompt — the agent's "personality" and output format |

The `GenericAgent` in `internal/core/domain/agent.go` is the **only** agent implementation. It is a universal runtime that executes behaviors defined by these files. This means:

- ✅ Create a new agent → drop config files, no recompilation
- ✅ Update an agent → edit `AGENT.md`, the LLM picks up the change
- ✅ Delete an agent → run `navi agent remove <id>`
- ✅ Share an agent → share two text files

## Agent Interface

All agents satisfy the `domain.Agent` interface:

```go
type Agent interface {
    ID() AgentID
    Config() AgentConfig
    Role() AgentRole
    IsTrusted() bool
    CanHandle(task Task) bool
    Execute(ctx context.Context, task Task) (TaskResult, error)
    CallTool(ctx context.Context, call ToolCall) (ToolResponse, error)
}
```

The orchestrator depends only on this interface. `GenericAgent` is the only concrete implementation.

## GenericAgent

`internal/core/domain/agent.go`

```go
type GenericAgent struct {
    config    AgentConfig
    llm       LLMPort       // injected by orchestrator via LLMFactory
    isolation IsolationPort  // injected by orchestrator via IsolationFactory
    inbox     chan AgentMessage
    outbox    chan AgentMessage
}
```

### Lifecycle

1. **`NewGenericAgent(cfg, llm, isolation)`** — constructs with injected adapters.  
2. **`Start(ctx)`** — starts background inbox goroutine.  
3. **`Execute(ctx, task)`** — builds prompt = system prompt + task, calls LLM, parses JSON, applies file changes via `IsolationPort`.  
4. **`Stop()`** — cancels the context, goroutine exits.

### Task Execution Flow

```
Orchestrator
    │
    ├─ FindIdle(task)        ← capability-based routing
    │
    ▼
GenericAgent.Execute(ctx, task)
    │
    ├─ buildPrompt()         ← systemPrompt + "\n\n---\n\nTask:\n" + task.Prompt
    ├─ llm.Generate(ctx, prompt)
    ├─ json.Unmarshal → ResultPayload
    └─ isolation.WriteFile() ← apply each FileChange
```

### LLM Retry

The agent retries transient LLM errors up to 3 times with linear backoff:

```go
for i := range 3 {
    resp, err = a.llm.Generate(ctx, prompt)
    if err == nil { return resp, nil }
    time.Sleep(time.Duration(i+1) * time.Second)
}
```

## Agent Configuration

### Directory Layout

```
~/.config/navi/agents/<agent-id>/
├── config.toml
└── AGENT.md           (or any file name declared in config.toml's `prompt` field)
```

### `config.toml` Reference

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `id` | string | ✅ | — | Unique ID, must match directory name |
| `type` | string | — | `"generic"` | Agent type (only `"generic"` exists) |
| `description` | string | — | — | Human-readable label |
| `prompt` | string | ✅ | — | Filename of system prompt (e.g. `"AGENT.md"`) |
| `llm.provider` | string | ✅ | — | `openai`, `ollama`, or any OpenAI-compatible API |
| `llm.model` | string | ✅ | — | Model name (e.g. `gpt-4o`) |
| `llm.api_key` | string | — | env `OPENAI_API_KEY` | API key (prefer env var) |
| `llm.base_url` | string | — | `https://api.openai.com/v1` | Override for Ollama, Together, etc. |
| `llm.temperature` | float | — | `0.7` | Sampling temperature |
| `llm.max_tokens` | int | — | `4096` | Max tokens to generate |
| `capabilities` | list(string) | ✅ | — | Granted capabilities (see below) |
| `isolation` | string | — | `"native"` | Isolation backend: `native`, `docker`, `bubblewrap` |
| `isolation_config` | table | — | — | Backend-specific: `image`, `memory`, `cpus` |
| `timeout` | duration | — | `"30m"` | Task timeout |
| `max_concurrent` | int | — | `5` | Max concurrent tasks |

### Capability Strings

Capabilities follow the format `<type>:<resource>:<mode>`:

| String | Meaning |
|---|---|
| `filesystem:workspace:rw` | Read+write access to the workspace directory |
| `filesystem:workspace:ro` | Read-only access |
| `exec:bash,go,git` | May execute these binaries (comma-separated) |
| `network:api.github.com:443` | May connect to this host on this port |
| `network:*:443` | May connect to any HTTPS host |
| `tool:mcp-name` | Access a local MCP server |
| `vision` | Enable multimodal vision input |
| `ocr:tesseract` | OCR via tesseract engine |
| `audio:whisper` | Audio input via Whisper |

### `AGENT.md` Format

Write the system prompt in Markdown or plain text. Always include an output format specification. The recommended format is JSON for structured results:

````markdown
You are an expert software developer.

## Constraints
- Only modify files inside the workspace.
- Never introduce security vulnerabilities.

## Output Format

```json
{
  "task_id": "<task id>",
  "output": "<description of changes>",
  "files": [
    {"path": "relative/path", "content": "..."}
  ],
  "success": true
}
```
````

## Isolation Backends

All agent side effects (file writes, command execution) go through `IsolationPort`.

| Backend | Binary | Security | Best For |
|---|---|---|---|
| `native` | none | Path restriction only | Local dev, read-only tasks |
| `docker` | `docker` | Full container isolation | VPS, multi-user, production |
| `bubblewrap` | `bwrap` | User namespace sandbox | Linux desktop (Arch, Fedora) |

The `IsolationPort` interface:

```go
type IsolationPort interface {
    Execute(ctx, cmd, args, env) (exitCode, stdout, stderr, err)
    ReadFile(ctx, path) (string, error)
    WriteFile(ctx, path, content) error
    Cleanup(ctx) error
}
```

Adapters live in `internal/adapters/isolation/{native,docker,bubblewrap}/`.

## Orchestrator & Agent Lifecycle

### Startup (`orchestrator.Start`)

1. `cfgReg.LoadAll()` — scans `~/.config/navi/agents/*/config.toml`
2. For each config: construct `GenericAgent`, call `agent.Start(ctx)`
3. Add to `InMemoryAgentRegistry`
4. Emit `agent.loaded` event

### Dynamic Registration (`navi agent create`)

1. Interactive wizard collects config fields
2. `LocalFSRegistry.Save(cfg)` writes `config.toml` + `AGENT.md` to disk
3. `Orchestrator.RegisterAgent(ctx, cfg)` starts the agent immediately
4. Emits `agent.created` event — TUI refreshes automatically

### Task Routing

The orchestrator uses capability-based routing:

```go
func (o *Orchestrator) Submit(ctx, task) (TaskResult, error) {
    agent, ok := registry.FindIdle(task) // finds agent whose caps ⊇ task.Requirements
    // ...
    return agent.Execute(ctx, task)
}
```

### Graceful Shutdown

On `SIGINT`/`SIGTERM`:
1. Root context cancelled
2. All agent goroutines exit their select loop
3. `Orchestrator.Shutdown()` calls `agent.Stop()` for all agents

## Agent Communication

Agents communicate only with the orchestrator. There is no direct agent-to-agent messaging.

```go
type AgentMessage struct {
    From    AgentID
    To      AgentID
    Type    string      // "request" | "response" | "event" | "error"
    Payload interface{} // TaskPayload | ResultPayload
}
```

All events are persisted to the SQLite `events` table for audit and replay.

## Memory & Context

### Short-Term (per-task)

```go
type TaskContext struct {
    TaskID        string
    Goal          string
    WorkspacePath string
    Capabilities  []Capability
    History       []AgentMessage
}
```

Context is not shared between concurrent tasks.

### Long-Term (planned)

A `VectorStore` port is defined in `ports/interfaces.go` for persistent memory:

```go
type VectorStore interface {
    Add(ctx, vector, metadata) (string, error)
    Search(ctx, query, limit) ([]SearchResult, error)
    Delete(ctx, id) error
}
```

## CLI Commands

```bash
# Create a new agent interactively
navi agent create

# List all registered agents
navi agent list

# Remove an agent
navi agent remove <agent-id>
```

## Testing

### Unit Tests (Capability Parser)

```bash
go test ./internal/core/services/capabilities/...
```

### Mock LLM

```go
type MockLLM struct{ Response string }
func (m *MockLLM) Generate(_ context.Context, _ string) (string, error) {
    return m.Response, nil
}
func (m *MockLLM) Stream(_ context.Context, _ string, chunk func(string)) error {
    chunk(m.Response); return nil
}

agent := domain.NewGenericAgent(cfg, &MockLLM{Response: `{"task_id":"t1","output":"ok","success":true}`}, nil)
result, err := agent.Execute(context.Background(), task)
```

### Integration Test Skeleton

```go
func TestGenericAgent_Integration(t *testing.T) {
    if testing.Short() { t.Skip() }
    apiKey := os.Getenv("OPENAI_API_KEY")
    llm, err := openai.New(apiKey, "gpt-4o-mini", "", 0.7, 512)
    if err != nil {
        t.Fatalf("failed to create LLM adapter: %v", err)
    }
    agent := domain.NewGenericAgent(testConfig, llm, native.New(nil))
    result, err := agent.Execute(context.Background(), domain.Task{
        ID:     "test-1",
        Prompt: "Say hello in JSON: {\"output\": \"hello\"}",
    })
    require.NoError(t, err)
    require.True(t, result.Completed)
}
```

## Observability

### Prometheus Metrics (planned)

```go
navi_agent_tasks_started_total{agent_id}   // counter
navi_agent_task_duration_seconds{agent_id} // histogram
```

### Structured Logging (planned)

```go
log.Info("task started",
    zap.String("agent_id", a.ID()),
    zap.String("task_id", taskID),
)
```

Levels: DEBUG (prompts), INFO (lifecycle), WARN (retries), ERROR (failures).

## Performance

| Resource | Typical | Notes |
|---|---|---|
| Memory per agent | ~100 KB | Prompt + config in-memory |
| LLM call overhead | 0.5–30s | Depends on provider and model |
| Docker startup | 500ms–2s | Use bubblewrap for <50ms |
| Max concurrent | 5 (default) | Configurable via `max_concurrent` |