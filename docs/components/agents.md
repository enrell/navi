# Agents

This document provides technical details on Navi's agent system. Agents are configurable workers defined by `config.toml` and `AGENT.md` files. The core provides a `GenericAgent` implementation; differentiation is via configuration, not code.

## Agent Interface

All agents must implement the domain `Agent` interface:

```go
package domain

type Agent interface {
    ID() string
    Type() string
    HandleMessage(ctx context.Context, msg AgentMessage) error
    // Additional lifecycle methods (Start, Stop) may be defined.
}
```

The orchestrator depends only on this interface, allowing multiple implementations. Currently, only `GenericAgent` is provided.

## GenericAgent

The `internal/agents/generic.go` package contains the `GenericAgent` struct:

```go
type GenericAgent struct {
    id        string
    config    AgentConfig
    prompt    string
    llm       ports.LLMPort
    isolation ports.IsolationPort
    logger    *zap.Logger
    inbox     chan ports.AgentMessage
}
```

It implements `domain.Agent` by:
- Reading the system prompt from `AGENT.md` at initialization.
- Using the configured LLM (via `ports.LLMPort`) to generate responses.
- Enforcing capabilities through the `ports.IsolationPort` when performing actions (e.g., file access, exec).
- Managing an inbox channel for incoming messages.

### Configuration Loading

During `NewGenericAgent(cfg AgentConfig)`, the agent:
1. Validates the `config.toml` schema.
2. Resolves the `prompt` file path relative to the agent directory and reads its content.
3. Establishes a connection to the LLM provider using the configured `llm.provider` and `api_key`.
4. Initializes the isolation backend based on `isolation` (docker, bubblewrap, native).
5. Starts the message handling loop in a goroutine.

### Message Handling

```go
func (a *GenericAgent) HandleMessage(ctx context.Context, msg domain.AgentMessage) error {
    // Only request messages are expected from orchestrator
    task := msg.Payload.(domain.TaskPayload)

    // Build full prompt: system + task + context
    fullPrompt := a.buildPrompt(task)

    // Generate response
    resp, err := a.llm.Generate(ctx, fullPrompt)
    if err != nil {
        return err
    }

    // Parse response (expecting structured JSON)
    var result domain.ResultPayload
    if err := json.Unmarshal([]byte(resp), &result); err != nil {
        return fmt.Errorf("failed to parse LLM response: %w", err)
    }

    // Send response back
    a.Send(domain.AgentMessage{
        To:     msg.From,
        Type:   "response",
        Payload: result,
    })

    return nil
}
```

## Agent Configuration

### Directory Layout

```
~/.config/navi/agents/<agent-name>/
├── config.toml
└── AGENT.md
```

### `config.toml` Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique agent identifier (must match directory name) |
| `type` | string | yes | Agent implementation type; currently only `"generic"` |
| `description` | string | no | Human-readable description |
| `prompt` | string | yes | Filename of system prompt (relative path) |
| `llm.provider` | string | yes | LLM provider: `openai`, `anthropic`, `ollama`, etc. |
| `llm.model` | string | yes | Model name (e.g., `gpt-4o`) |
| `llm.temperature` | float | no | Sampling temperature (default 0.7) |
| `llm.max_tokens` | int | no | Max tokens to generate (default 4096) |
| `capabilities` | list(string) | yes | List of granted capabilities |
| `isolation` | string | yes | Isolation backend: `docker`, `bubblewrap`, `native` |
| `isolation_config` | table | no | Backend-specific options (image, memory, cpus) |
| `timeout` | duration | no | Task timeout (default 30m) |
| `max_concurrent` | int | no | Max concurrent tasks (default 5) |

### Capability Strings

Capabilities are expressed as:

- `filesystem:<path>:<mode>` – `<mode>` is `ro` or `rw`.
- `network:<host>:<port>` – Use `*` for any host/port (dangerous, requires extra auth).
- `exec:<binary1>,<binary2>` – Allowed executable names.
- `vision` – Enable multimodal vision (if model supports).
- `ocr:<engine>` – OCR engine (tesseract, easyocr).
- `audio:<mode>` – Audio input/output (whisper, tts).
- `tool:<name>` – External tool via MCP.

Example: `["filesystem:workspace:rw", "exec:bash,go", "network:api.github.com:443"]`.

### `AGENT.md`

The system prompt. Write in Markdown or plain text. Use clear instructions. Include output format specification (JSON recommended). Example for a coder agent:

````markdown
You are an expert software developer.

Your task: Write, review, and refactor code in multiple languages.

Constraints:
- Only modify files inside the workspace.
- Include unit tests with >80% coverage.
- Follow idiomatic style for the language.
- Never introduce security vulnerabilities.
- Output JSON: {"files": [{"path": "relative/path", "content": "..."}]}.

When given a task, produce the requested changes.
````

## Agent Lifecycle

### Startup

At orchestrator startup:
1. Scan `~/.config/navi/agents/*/config.toml`.
2. For each valid config, create a `GenericAgent`.
3. Store in the `AgentRegistry` keyed by `agent.ID`.
4. Start each agent's message loop goroutine.
5. Log `agent.loaded` event.

### Dynamic Registration

The `navi agent create` CLI:
- Prompts for configuration (or reads flags).
- Requires user authentication (password/biometric) before writing files.
- Writes `config.toml` and `AGENT.md` to `~/.config/navi/agents/<name>/`.
- If orchestrator is running, sends an authenticated `POST /agents/register` request.
- Orchestrator validates the config, registers the agent, and emits `agent.created` event.
- TUI updates automatically.

### Task Execution Flow

1. User submits a task via TUI/API/CLI.
2. Orchestrator determines required capabilities from the task (either from task metadata or by analyzing with a meta-agent).
3. Orchestrator selects an idle agent that has all required capabilities (capability-based routing).
4. Orchestrator sends `AgentMessage{Type: "request", Payload: TaskPayload}` to the agent's inbox.
5. Agent processes the request:
   - Constructs a prompt combining system prompt, task description, and any relevant context.
   - Calls its LLM.
   - Parses the response into structured data.
   - May use capabilities (e.g., file operations) via isolation.
   - Sends back `AgentMessage{Type: "response", Payload: ResultPayload}`.
6. Orchestrator receives the response, logs the outcome, and returns the result to the user.

### Shutdown

On orchestrator termination (SIGINT/SIGTERM):
- Cancel the root context.
- All agents' message loops exit.
- Wait for agent goroutines to finish.
- Close isolation backends.

## Agent Communication

Agents communicate only with the orchestrator. Direct agent-to-agent messaging is not supported; all communication goes through the orchestrator, which routes messages based on `To` field.

```go
type AgentMessage struct {
    From    string
    To      string
    Type    string // "request", "response", "event", "error"
    Payload interface{}
}
```

### Persistence

All messages are logged to the event store:

```sql
INSERT INTO event_log (agent_id, action, details)
VALUES ('agent-id', 'message_sent', '{"to":"orchestrator","type":"response","size":1234}');
```

This enables replay, audit, and debugging.

## Memory & Context

### Short-Term Context

Each task receives its own context:

```go
type TaskContext struct {
    TaskID       string
    Goal         string
    Workspace    Workspace
    Capabilities Capabilities  // Granted to this task
    History      []AgentMessage // Previous messages in the conversation
}
```

Context is not shared between concurrent tasks of the same agent.

### Long-Term Memory (Planned)

A vector store will be available for agents to store and retrieve persistent knowledge:

```go
type Memory interface {
    Remember(ctx context.Context, key string, value []byte) error
    Recall(ctx context.Context, query string, limit int) ([]MemoryItem, error)
}
```

Use cases: user preferences, past results, learned facts.

## Error Handling

The orchestrator expects agents to return errors rather than panic. Panics are caught and converted to error messages.

### Agent-Side

```go
func (a *GenericAgent) HandleMessage(ctx context.Context, msg domain.AgentMessage) error {
    defer func() {
        if r := recover(); r != nil {
            a.logger.Error("panic in agent", zap.Any("panic", r))
        }
    }()

    // ... processing ...

    if err != nil {
        return fmt.Errorf("agent %s: %w", a.ID(), err)
    }
    return nil
}
```

### Orchestrator-Side

Orchestrator may retry on transient errors:

```go
for i := 0; i < maxRetries; i++ {
    err := agent.HandleMessage(ctx, msg)
    if err == nil {
        return nil
    }
    if !isTransient(err) {
        break
    }
    time.Sleep(backoff(i))
}
return ErrTaskFailed
```

## Retry Logic

Agents themselves can implement retries when calling external services (LLM APIs, HTTP). Example:

```go
func (a *GenericAgent) callLLMWithRetry(ctx context.Context, prompt string) (string, error) {
    var resp string
    var err error
    for i := 0; i < 3; i++ {
        resp, err = a.llm.Generate(ctx, prompt)
        if err == nil {
            return resp, nil
        }
        if !isTransient(err) {
            return "", err
        }
        time.Sleep(time.Duration(i+1) * time.Second)
    }
    return "", fmt.Errorf("max retries exceeded: %w", err)
}
```

## Observability

### Metrics (Prometheus)

```go
var (
    tasksStarted = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "navi_agent_tasks_started_total",
            Help: "Total number of tasks started per agent",
        },
        []string{"agent_id"},
    )
    tasksCompleted = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "navi_agent_task_duration_seconds",
            Help:    "Task duration by agent",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
        },
        []string{"agent_id"},
    )
)
```

Record at start and completion.

### Logging

Use structured logging with agent ID and task ID:

```go
log := a.logger.With(
    zap.String("agent_id", a.ID()),
    zap.String("task_id", taskID),
)
log.Info("task started")
```

Levels:
- DEBUG: full prompts and LLM responses (avoid logging secrets).
- INFO: task lifecycle events.
- WARN: retries, partial failures.
- ERROR: task failure, crashes.

### Tracing

Future: OpenTelemetry integration to trace requests across agents.

## Performance Considerations

- **Parallelism**: Agents run in separate goroutines; ensure `max_concurrent` per agent is enforced.
- **LLM rate limits**: If using shared API keys, implement per-agent rate limiting (token bucket).
- **Memory**: Each agent holds its prompt and some context; ~100KB per agent typical.
- **Isolation overhead**: Docker containers start slower; bubblewrap faster; native fastest but less secure.

## Testing Agents

### Unit Testing with Mock LLM

```go
type MockLLM struct {
    responses []string
    mu        sync.Mutex
}

func (m *MockLLM) Generate(ctx context.Context, prompt string) (string, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if len(m.responses) == 0 {
        return "", errors.New("no more responses")
    }
    resp := m.responses[0]
    m.responses = m.responses[1:]
    return resp, nil
}
```

Test:

```go
func TestGenericAgent(t *testing.T) {
    cfg := AgentConfig{ /* minimal valid config */ }
    llm := &MockLLM{responses: []string{`{"result":"ok"}`}}
    agent := agents.NewGenericAgent(cfg, llm)

    msg := domain.AgentMessage{
        Type: "request",
        Payload: domain.TaskPayload{Description: "test"},
    }

    err := agent.HandleMessage(context.Background(), msg)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // Verify LLM was called, response sent, etc.
}
```

### Integration Testing

```go
func TestGenericAgent_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("integration test")
    }
    cfg := loadTestAgentConfig("coder")
    llm := adapters.NewOpenAIAdapter(adapters.OpenAIConfig{
        APIKey: os.Getenv("OPENAI_API_KEY"),
    })
    defer llm.Close()
    agent := agents.NewGenericAgent(cfg, llm)

    // Simulate a coding task...
}
```

### Load Testing

Simulate many concurrent tasks:

```go
func BenchmarkAgent_Parallel(b *testing.B) {
    cfg := loadTestAgentConfig("generic")
    llm := &MockLLM{responses: make([]string, b.N)}
    for i := range llm.responses {
        llm.responses[i] = `{"result":"ok"}`
    }
    agent := agents.NewGenericAgent(cfg, llm)

    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            msg := domain.AgentMessage{Type: "request", Payload: domain.TaskPayload{Description: "bench"}}
            agent.HandleMessage(context.Background(), msg)
        }
    })
}
```

## Example Agent Configurations

### Research Agent

`config.toml`:

```toml
id = "researcher"
type = "generic"
description = "Web research"
prompt = "AGENT.md"
llm.provider = "openai"
llm.model = "gpt-4o"
capabilities = ["network:*", "websearch"]
isolation = "docker"
```

`AGENT.md`:

```markdown
You are a Research Agent. Use web search to answer queries. Evaluate sources, cite URLs, and provide a confidence level. Output JSON with findings and synthesis.
```

### Coding Agent

`config.toml`:

```toml
id = "coder"
type = "generic"
description = "Go code generation"
prompt = "AGENT.md"
llm.provider = "openai"
llm.model = "gpt-4o"
capabilities = ["filesystem:workspace:rw", "exec:bash,go,git"]
isolation = "docker"
isolation_config = { image = "navi-base:latest", memory = "1g" }
```

`AGENT.md`:

```markdown
You are an expert Go developer. Write clean, idiomatic Go code with tests. Follow existing project patterns. Output JSON with file paths and contents.
```

## Security Considerations

- **Capability Grants**: Always use least privilege. Avoid `network:*`, `filesystem:/:rw`, `exec:*`.
- **Prompt Injection**: LLMs may ignore prompts; isolation is the final barrier.
- **Agent Creation Auth**: Agent creation via the TUI requires user authentication to prevent malicious scripts from adding agents.
- **Config Validation**: Orchestrator should validate configs on load and reject those with overly broad capabilities unless an explicit `dangerous = true` flag is set (which requires additional auth).
- **Audit Logging**: All agent creation and modification events are logged with user ID and full config snapshot for traceability.
- **Secret Management**: Never put API keys in agent prompts; use global config with environment variable expansion.

## Future Directions

- **Memory**: Vector store integration for long-term recall.
- **Tool Use**: MCP (Model Context Protocol) for calling external tools.
- **Delegation**: Agents may spawn sub-agents with reduced capabilities.
- **Learning**: Fine-tune agent behavior based on task outcomes.
- **Agent Marketplace**: Share and discover community agents.

## References

- [Agent System Overview](../../AGENTS.md)
- [Hexagonal Architecture](../../architecture/overview.md)
- [Isolation Adapters](./isolation-adapters.md)
