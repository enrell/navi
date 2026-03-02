# Architecture Overview

Navi is built on **Hexagonal Architecture** (Ports & Adapters) with a **GenericAgent Runtime Engine** at its core. Agents, tools, and skills are **pure data** (`config.toml` + `AGENT.md`) — the engine executes them without recompilation.

## Architecture Principles

### 1. Hexagonal Architecture

The AI landscape is volatile. New models emerge, APIs change, and frameworks evolve. Hexagonal architecture protects the core business logic from these external changes by:

1. **Isolating the core** from external concerns
2. **Defining clear contracts** via interfaces (ports)
3. **Enabling swappable implementations** via adapters
4. **Making testing easy** by mocking ports

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

### 2. Agents Are Data

Every agent is defined by two files:

```
~/.config/navi/agents/<name>/
├── config.toml   ← LLM, capabilities, isolation backend
└── AGENT.md      ← system prompt (the agent's "brain")
```

Create any specialist (researcher, coder, planner, security auditor…) by writing config files — **no recompilation**.

### 3. Default Agents

Default agents are stored in `configs/agents/` in the repository. During installation, a setup script pulls these defaults from the GitHub repository. Users can customize them in their local `~/.config/navi/agents/` directory.

## Directory Structure

```
cmd/navi/              — entry point; CLI dispatch + wiring
internal/
  core/
    domain/            — Agent interface, GenericAgent, AgentConfig, types
    ports/             — LLMPort, IsolationPort, AgentConfigRegistry, EventLog
    services/
      orchestrator/    — capability-based routing, agent lifecycle
      capabilities/    — capability string parser + Satisfies matcher
  adapters/
    llm/openai/        — OpenAI REST adapter (no external SDK)
    isolation/
      native/          — host OS with path restriction
      docker/          — ephemeral container via docker run
      bubblewrap/      — bwrap user namespace sandbox
    registry/localfs/  — reads/writes ~/.config/navi/agents/*/
    storage/sqlite/    — event log + agent persistence (GORM)
  ui/
    repl/              — terminal REPL
    api/               — REST API server
pkg/                   — shared utilities
configs/agents/        — default agents (embedded in binary)
  orchestrator/
  coder/
  researcher/
```

## Entry Points & Communication

Navi supports multiple entry points, each using the appropriate communication protocol:

| Entry Point | Communication | Security |
|-------------|---------------|----------|
| **TUI** (local) | gRPC via Unix Domain Socket (`/tmp/navi.sock`) | Unix file permissions |
| **Web UI** (remote) | REST (grpc-gateway) + WebSocket | SRP + Token + HTTPS |
| **API/Bots** (remote) | REST | API Keys / PASETO + HTTPS |
| **Desktop App** | gRPC or REST (local) | Token saved in OS Keyring |

### Why Unix Domain Sockets?

For local communication, Navi uses **Unix Domain Sockets** instead of TCP:

- **Faster**: Zero network overhead, direct kernel-to-kernel
- **More Secure**: Protected by Unix file permissions — only processes with read access can connect
- **No MitM**: Local-only, impossible to intercept from another machine

```
# Socket location
/tmp/navi.sock

# Permissions (example)
srw-rw---- 1 enrell enrell 0 /tmp/navi.sock
```

### Why gRPC + REST + WebSocket?

- **gRPC**: Native streaming, efficient binary protocol, ideal for local + real-time
- **REST**: Standard HTTP, easy to proxy, great for CRUD operations
- **WebSocket**: Full-duplex communication for LLM streaming in browsers

## Core Components

### 1. Orchestrator (The Maestro)

`internal/core/services/orchestrator/orchestrator.go`

The orchestrator is the runtime hub. It:
- Loads all agent configs from `~/.config/navi/agents/` on startup
- Creates a `GenericAgent` for each config with injected LLM and Isolation adapters
- Routes tasks to agents by **capability matching** (`agent.caps ⊇ task.required`)
- Supports **hot-registration**: `navi agent create` adds an agent at runtime without restart
- Persists all events to the SQLite event log

### 2. GenericAgent (The Universal Runtime)

`internal/core/domain/agent.go`

There is **one** agent implementation. Its behavior is entirely driven by `config.toml` + `AGENT.md`.

```go
type GenericAgent struct {
    config    AgentConfig   // loaded from config.toml + AGENT.md
    llm       LLMPort       // adapter injected by Orchestrator
    isolation IsolationPort // adapter injected by Orchestrator
    inbox     chan AgentMessage
    outbox    chan AgentMessage
}
```

Create any specialist (researcher, coder, planner, etc.) by writing two files — no Go code required.

### 3. Capability System

`internal/core/services/capabilities/`

Capabilities are strings that describe exactly what an agent may do:

| String | Grants |
|---|---|
| `filesystem:workspace:rw` | Read+write the workspace dir |
| `exec:bash,go,git` | Run these binaries |
| `network:api.github.com:443` | Connect to this HTTPS endpoint |
| `tool:mcp-name` | Call a local MCP server |

The capability parser tokenises strings; `Satisfies(agentCaps, required)` handles routing.

### 4. Ports (Interfaces)

`internal/core/ports/interfaces.go`

| Port | Purpose | Adapters |
|---|---|---|
| `LLMPort` | Call any language model | `adapters/llm/openai` (+ Ollama-compat) |
| `IsolationPort` | Safe command/file execution | `isolation/native`, `docker`, `bubblewrap` |
| `AgentConfigRegistry` | Load/save agent configs from disk | `registry/localfs` |
| `EventLog` | Persist all events | `storage/sqlite` |
| `VectorStore` | Long-term semantic memory (planned) | — |

```go
// core ports (abridged)
type LLMPort interface {
    Generate(ctx context.Context, prompt string) (string, error)
    Stream(ctx context.Context, prompt string, chunk func(string)) error
    Health(ctx context.Context) error
}

type IsolationPort interface {
    Execute(ctx, cmd, args, env) (exitCode, stdout, stderr, err)
    ReadFile(ctx, path) (string, error)
    WriteFile(ctx, path, content) error
    Cleanup(ctx) error
}

type AgentConfigRegistry interface {
    LoadAll() ([]domain.AgentConfig, error)
    Save(cfg domain.AgentConfig) error
    Delete(id string) error
}
```

### 5. Adapters (Implementations)

Adapters implement ports for specific technologies. They handle:
- API authentication
- Format conversions
- Error translation
- Connection management

Example: `OpenAIAdapter` implements `LLMPort` by wrapping the official OpenAI Go client and translating Navi's `Prompt` type to OpenAI's request format.

## Security Boundaries

### Isolation Layers

1. **Process Isolation**: Each task runs in its own sandbox (Docker container, Bubblewrap sandbox, or restricted native process)
2. **Filesystem Isolation**: Only mounted workspace (and explicit approved paths) are accessible
3. **Network Isolation**: Only whitelisted endpoints can be reached
4. **Capability Enforcement**: Each operation is explicitly granted/denied
5. **Authentication**: Every request must be authenticated

### Local Communication Security

For TUI and local access, Navi uses **Unix Domain Sockets** with file permissions:

```
/tmp/navi.sock — Only accessible to the owner
```

This provides:
- **Process-level isolation**: Only users with socket permission can connect
- **No network exposure**: Not even localhost:port is opened
- **OS-enforced security**: Relies on Unix permissions, not application code

### Remote Communication Security

For Web UI and API access:

| Layer | Technology |
|-------|-----------|
| Authentication | SRP (Secure Remote Password) + Token Opaco |
| Transport | HTTPS / mTLS |
| Real-time | WebSocket with token in initial handshake |

### Threat Model Addressed

| Threat | Mitigation |
|--------|-----------|
| Accidental `rm -rf /` | Workspace is isolated mount; host filesystem inaccessible |
| Unintended network access | Network whitelist enforced by isolation backend |
| Credential leakage | Credentials never passed to agents; access via capability grants |
| Prompt injection | Auth checks before every operation; no trusted input |
| Privilege escalation | Agents run with minimal privileges; no root unless explicitly granted |
| Local MitM attack | Unix Domain Socket — no network exposure |

## Agent Sync System

### The Navi Solution: Hybrid Storage Model

Navi uses **filesystem as the interface** and **SQLite as the validator** (Checksum Store).

#### How It Works

**Storage**: Agents live in `~/.config/navi/agents/` as `.toml` and `.md` files.

**Validation (SQLite)**: Navi maintains an internal SQLite containing only:
- `agent_id` - Unique identifier
- `path` - File path
- `file_hash` - SHA-256 hash of the agent files
- `signature` - Cryptographic signature (optional, for verified agents)
- `status` - Trusted / Untrusted / Modified

#### Edit Flow

1. **Via Interface**: When editing through Navi's interface (with hardware key/password), Navi updates the file on disk and generates a new hash in SQLite.

2. **Manual Edit (Neovim, etc.)**: If you edit manually:
   - Navi detects the change (via fsnotify or on boot)
   - Prompts: *"Agent X was manually modified. Do you want to authorize the new capabilities with your key?"*
   - User authenticates via SRP to validate

#### Injection Blocking

If a new agent appears in the folder **without** a corresponding SQLite record:
- Navi marks it as **Untrusted**
- Refuses to load it until explicitly validated via authenticated interface (SRP)

This prevents malicious agents from being injected into the system.

### Default Agents Flow

```
configs/agents/ (repository)
        │
        ▼ git pull / install
~/.config/navi/agents/ (user local)
        │
        ▼ fsnotify / boot check
~/.config/navi/agents.db (SQLite validator)
        │
        ▼ hash validation
    Trusted / Untrusted / Modified
```

### Installation Flow

1. User runs `navi setup` or installs via package manager
2. Default agents are placed in `~/.config/navi/agents/`
3. SQLite database created with agent records
4. Default agents marked as **Trusted**

### Update Flow

1. Navi starts and checks for agent changes (fsnotify)
2. If changes detected: prompt user *"Update agents? (y/N)"*
3. If approved: API endpoint `POST /agents/sync` pulls latest from GitHub
4. New hash calculated and stored in SQLite
5. Agents hot-reloaded without restart

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

## Key Design Decisions

### Why Not a Plugin Marketplace?

External plugin marketplaces (npm-style, VS Code extensions) open vectors for supply-chain attacks. Navi's approach:
- **Agents are data** — `config.toml` + `AGENT.md` files, not compiled code
- **Adapters are compiled-in** — type-safe, auditable, version-compatible
- **No external plugin registry** — no unknown code runs inside the process
- **MCP for external tools** — run untrusted tool servers in their own sandbox via `tool:<name>` capability

### Why Go?

- **Performance**: Compiled, concurrent, efficient
- **Simplicity**: Explicit, readable, minimal magic
- **Binary deployment**: One binary, no runtime
- **Ecosystem**: Excellent standard library, great for CLIs and servers
- **Tooling**: `go test`, `go fmt`, `go vet` are excellent
- **gRPC support**: Native, first-class

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
- [Unix Domain Sockets - Linux man pages](https://man7.org/linux/man-pages/man7/unix.7.html)
- [gRPC](https://grpc.io/)
