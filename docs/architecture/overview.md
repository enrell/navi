# Architecture Overview

Navi is built on **Hexagonal Architecture** (also known as Ports & Adapters) with a **Multi-Agent Agency** core. This document explains the architectural decisions and their rationale.

## Why Hexagonal Architecture?

The AI landscape is volatile. New models emerge, APIs change, and frameworks evolve. Hexagonal architecture protects the core business logic from these external changes by:

1. **Isolating the core** from external concerns
2. **Defining clear contracts** via interfaces (ports)
3. **Enabling swappable implementations** via adapters
4. **Making testing easy** by mocking ports

### The Problem Hexagonal Solves

Without hexagonal architecture, you might write:

```go
// BAD: Core logic tied to specific providers
func ProcessRequest(input string) (string, error) {
    // Direct OpenAI API call
    resp, err := openaiClient.CreateCompletion(input)
    if err != nil {
        return "", err
    }

    // Direct Docker command
    cmd := exec.Command("docker", "run", "...")
    output, err := cmd.Output()
    // ...
}
```

This makes testing impossible, changing providers painful, and introduces tight coupling.

### The Hexagonal Solution

```
┌─────────────────────────────────────────────┐
│                  ADAPTERS                   │
│  (OpenAIAdapter, DockerAdapter, SQLiteRepo) │
├─────────────────────────────────────────────┤
│                CORE LOGIC                   │
│        (Orchestrator, Agent, Planner)       │
│        Depends only on:                     │
│         - LLMPort                           │
│         - IsolationPort                     │
│         - RepositoryPort                    │
├─────────────────────────────────────────────┤
│                    PORTS                    │
│ (Interfaces: LLMPort, IsolationPort, etc.)  │
└─────────────────────────────────────────────┘
```

The core only knows about interfaces. Adapters implement those interfaces. Entry points (CLI, TUI, API) use the core.

## Core Components

### 1. Orchestrator (The Maestro)

The orchestrator is the central brain that coordinates agents, manages workflows, and enforces security policies.

Responsibilities:
- Receive user requests
- Delegate to appropriate agents
- Coordinate parallel execution
- Aggregate results
- Enforce capability constraints
- Audit all actions

### 2. Agency (Multi-Agent System)

Navi uses **specialized agents** rather than a single monolithic "god agent."

#### Agent Types

| Agent | Responsibility | Context Scope |
|-------|----------------|---------------|
| **Planner** | Breaks down tasks into steps | High-level goals only |
| **Researcher** | Gathers information, searches | External data sources |
| **Coder** | Writes, reviews, refactors code | Codebase + documentation |
| **Executor** | Runs tools, APIs, scripts | Execution environment |
| **Verifier** | Validates outputs, catches errors | Expected outcomes + constraints |

**Key Principle**: Each agent has a **single, focused responsibility**. They communicate via a well-defined protocol, not by sharing memory.

#### Why Specialization Works

LLMs suffer from **context dilution**—around token 8,000 on a 32K context model, they start ignoring early instructions. By splitting work among specialized agents:

- Each agent has a **smaller cognitive scope**
- Instructions are clearer and less conflicting
- Hallucinations are contained and caught by verifiers
- Failures are isolated, not catastrophic

### 3. Ports (Interfaces)

Ports define contracts that the core depends on. They are pure Go interfaces.

#### LLMPort

```go
type LLMPort interface {
    Generate(ctx context.Context, prompt Prompt) (Response, error)
    Embed(ctx context.Context, text string) (Vector, error)
    Stream(ctx context.Context, prompt Prompt) (<-chan Token, error)
}
```

Implemented by: `OpenAIAdapter`, `AnthropicAdapter`, `OllamaAdapter`, etc.

#### IsolationPort

```go
type IsolationPort interface {
    Execute(ctx context.Context, cmd Command, caps Capabilities) (Result, error)
    FileRead(ctx context.Context, path string) ([]byte, error)
    FileWrite(ctx context.Context, path string, data []byte) error
    Mount(ctx context.Context, source, target string, readonly bool) error
}
```

Implemented by: `DockerAdapter`, `BubblewrapAdapter`, `NativeAdapter`.

#### RepositoryPort

```go
type RepositoryPort interface {
    SaveEvent(ctx context.Context, event Event) error
    GetHistory(ctx context.Context, filter HistoryFilter) ([]Event, error)
    SaveWorkspaceState(ctx context.Context, workspace Workspace) error
    GetWorkspaceState(ctx context.Context, id string) (Workspace, error)
}
```

Implemented by: `SQLiteRepository`, `PostgresRepository`.

#### AuthPort

```go
type AuthPort interface {
    ValidateToken(ctx context.Context, token string) (User, error)
    CreateSession(ctx context.Context, user User) (Session, error)
    RevokeSession(ctx context.Context, sessionID string) error
    CheckPermission(ctx context.Context, user User, resource string, action string) bool
}
```

Implemented by: `LocalAuthAdapter`, `OAuthAdapter`.

### 4. Adapters (Implementations)

Adapters implement ports for specific technologies. They handle:
- API authentication
- Format conversions
- Error translation
- Connection management

Example: `OpenAIAdapter` implements `LLMPort` by wrapping the official OpenAI Go client and translating Navi's `Prompt` type to OpenAI's request format.

### 5. Entry Points (CLI, TUI, API)

Entry points are the user-facing interfaces. They:
- Parse user input
- Call the orchestrator
- Display results
- Handle authentication

Multiple entry points can coexist because the core is UI-agnostic.

## Data Flow

### Typical Request Flow

```
User → CLI/TUI/API → Auth Middleware → Orchestrator → Planner Agent
   → [Researcher + Coder in parallel] → Executor → Verifier
   → Result aggregation → Response to user
   → Audit log entry written
```

### Capability Enforcement

Every operation goes through the capability checker:

```go
func (o *Orchestrator) ExecuteTask(ctx context.Context, task Task) (Result, error) {
    user := auth.UserFromContext(ctx)

    // Check if user has permission for this task type
    if !o.auth.CheckPermission(user, task.Type, "execute") {
        return Result{}, ErrPermissionDenied
    }

    // Translate task capabilities to isolation constraints
    caps := o.translateCapabilities(task.Capabilities)

    // Execute with enforced constraints
    result, err := o.isolation.Execute(ctx, task.Command, caps)
    if err != nil {
        o.logger.Log("execution_failed", user.ID, task.ID, err)
        return Result{}, err
    }

    // Audit everything
    o.repository.SaveEvent(ctx, Event{
        UserID:    user.ID,
        Action:    "task_executed",
        TaskID:    task.ID,
        Result:    result,
    })

    return result, nil
}
```

## Multi-Agent Coordination

### Task Delegation Flow

```
┌─────────────┐
│   User      │ "Build a REST API with auth"
└──────┬──────┘
       │
       ▼
┌─────────────────────┐
│   Orchestrator      │
└──────┬──────────────┘
       │
       ├───────────────┐
       │ Planner Agent │ (breaks down task)
       └───────────────┘
              │
              ├─► Research auth patterns ──► Researcher
              ├─► Design API structure   ──► Planner (again)
              ├─► Implement endpoints    ──► Coder (parallel)
              ├─► Write tests            ──► Coder + Verifier
              └─► Build & run            ──► Executor
```

### Parallel Execution

Navi executes independent tasks concurrently:

```go
func (o *Orchestrator) ExecutePlan(ctx context.Context, plan Plan) (PlanResult, error) {
    var wg sync.WaitGroup
    results := make(chan TaskResult, len(plan.Steps))

    for _, step := range plan.Steps {
        if step.DependenciesMet(plan) {
            wg.Add(1)
            go func(s Task) {
                defer wg.Done()
                res := o.executeStep(ctx, s)
                results <- res
            }(step)
        }
    }

    wg.Wait()
    close(results)
    return aggregate(results), nil
}
```

## Security Boundaries

### Isolation Layers

1. **Process Isolation**: Each task runs in its own sandbox (Docker container, Bubblewrap sandbox, or restricted native process)
2. **Filesystem Isolation**: Only mounted workspace (and explicit approved paths) are accessible
3. **Network Isolation**: Only whitelisted endpoints can be reached
4. **Capability Enforcement**: Each operation is explicitly granted/denied
5. **Authentication**: Every request must be authenticated

### Threat Model Addressed

| Threat | Mitigation |
|--------|-----------|
| Accidental `rm -rf /` | Workspace is isolated mount; host filesystem inaccessible |
| Unintended network access | Network whitelist enforced by isolation backend |
| Credential leakage | Credentials never passed to agents; access via capability grants |
| Prompt injection | Auth checks before every operation; no trusted input |
| Privilege escalation | Agents run with minimal privileges; no root unless explicitly granted |

## Storage Architecture

### Event Sourcing

All actions are logged as immutable events:

```sql
CREATE TABLE event_log (
    id INTEGER PRIMARY KEY,
    timestamp TEXT NOT NULL,
    user_id TEXT NOT NULL,
    agent_id TEXT,
    action TEXT NOT NULL,
    capability TEXT,
    workspace_path TEXT,
    git_commit TEXT,
    result TEXT,
    error TEXT
);
```

This enables:
- Full audit trail
- Debugging and post-mortem analysis
- Compliance reporting
- Reproducibility (replay events to reconstruct state)

### Workspace State

Workspaces track:
- Current files (via git)
- In-progress tasks
- Agent session state
- Checkpoints for rollback

## Communication Protocols

### Agent-to-Agent

Agents communicate via a message bus:

```go
type AgentMessage struct {
    From    string      // Agent ID
    To      string      // Agent ID or "orchestrator"
    Type    string      // "request", "response", "event"
    Payload interface{} // Typed payload
}

// Orchestrator routes messages based on To field
```

Messages are persisted to the event log for observability.

### User-to-Orchestrator

Different entry points use different protocols:
- **CLI/TUI**: Direct function calls (same process)
- **REST API**: HTTP POST/GET with JWT
- **gRPC**: Bidirectional streaming for real-time updates
- **Bots**: Webhooks or long-polling

All require authentication and capability checks.

## Scalability Considerations

### Horizontal Scaling

The orchestrator can be distributed:
- Multiple orchestrator instances behind a load balancer
- Shared database (SQLite doesn't support clustering, so Postgres for distributed)
- Message queue (RabbitMQ, NATS) for inter-agent communication
- Agent workers can run on separate machines

Currently, Navi is single-node, but architecture supports future distribution.

### Performance Optimizations

- **Caching**: LLM responses can be cached by prompt hash
- **Checkpointing**: Long-running tasks can be paused and resumed
- **Streaming**: LLM responses streamed to UI as they arrive
- **Lazy loading**: Heavy adapters loaded on-demand

## Observability

### Metrics to Track

- Task queue depth
- Agent latency (per agent type)
- LLM token usage
- Adapter errors
- Capability denials
- Workspace changes

### Logging Strategy

- **Structured logging**: JSON logs with fields (`timestamp`, `level`, `component`, `user_id`, `task_id`)
- **Correlation IDs**: Every request gets ID, propagated through all logs
- **Sensitive data redaction**: Never log API keys, tokens, or private data

## Future Extensions

The hexagonal architecture makes adding new components straightforward:

### Adding a New LLM Provider

1. Implement `LLMPort` in `internal/adapters/<provider>_adapter.go`
2. Register it in the factory
3. Configure via `config.yaml`

No core changes needed.

### Adding a New Interaction Mode

1. Create a new package in `cmd/` or `internal/bots/`
2. Call the orchestrator via its public API
3. Handle authentication and UI

Again, no core changes needed.

## Key Design Decisions

### Why Not a Plugin Architecture?

Plugins can introduce stability and security risks. Adapters are compiled into the binary, ensuring:
- Type safety
- Auditability
- Version compatibility
- No runtime dependency surprises

### Why Go?

- **Performance**: Compiled, concurrent, efficient
- **Simplicity**: Explicit, readable, minimal magic
- **Binary deployment**: One binary, no runtime
- **Ecosystem**: Excellent standard library, great for CLIs and servers
- **Tooling**: `go test`, `go fmt`, `go vet` are excellent

### Why SQLite?

- Zero-configuration
- Single file, easy backup
- Good enough for single-node use
- ACID transactions ensure audit log integrity
- Pure Go implementations available (modernc, go-sqlite3)

Future: Postgres for distributed deployments.

## References

- [Hexagonal Architecture](https://alistair.cockburn.us/hexagonal-architecture/)
- [Clean Architecture by Robert C. Martin](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Ports and Adapters pattern](https://en.wikipedia.org/wiki/Hexagonal_architecture_(software))
