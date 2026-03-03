# Navi CLAUDE.md - Project Context

This file provides project context for AI assistants.

## Project Overview

Navi is a secure AI orchestrator built with hexagonal architecture. Agents are defined by config files (`config.toml` + `AGENT.md`), not hardcoded.

Current focus is **Sprint 1 runtime stability** (REPL/TUI loop, orchestrator tool-calling, MCP path, and local-dev ergonomics).

## Architecture

- **Hexagonal Architecture** with ports & adapters
- **Orchestrator**: Manages agent lifecycle and task routing
- **Agents**: Execute tasks based on capabilities
- **Isolation**: Sandboxed execution (native, docker, bubblewrap)

### Important Current Runtime Note

The current orchestrator implementation is intentionally minimal:
- model-driven tool loop (`TOOL_CALL` protocol)
- native tool registry
- basic in-process MCP integration
- clear REPL trace output sections (user/thinking/tool/orchestrator)

This is a foundation, not full multi-agent agency yet.

## Key Components

| Component | Path |
|-----------|------|
| CLI Entry | `cmd/navi/main.go` |
| Orchestrator | `internal/core/services/orchestrator/` |
| Domain | `internal/core/domain/` |
| Adapters | `internal/adapters/` |
| CLI Commands (repl/chat/serve) | `cmd/navi/cmd/` |
| HTTP Adapter | `internal/adapters/http/` |
| Local Agent Registry Loader | `internal/adapters/registry/localfs/` |
| SQLite Repositories | `internal/adapters/storage/sqlite/` |

## Entry Points

| Command | Description | Protocol |
|---------|-------------|----------|
| `navi` | CLI root command (subcommands) | Direct |
| `navi serve` | REST API server | HTTP |
| `navi repl` | Terminal REPL | Direct |
| `navi chat <msg>` | Single chat message | Direct |

**Planned:**
- `navi web` - Web UI
- Desktop App

## Development Phases

### Phase 1: REST API First (Current)

During initial development, the REST API is the **primary interface** for interacting with Navi. This approach:
- **Facilitates rapid development**: External tools and scripts can easily integrate
- **Simplifies debugging**: HTTP is easier to inspect than terminal UI
- **Enables automation**: CI/CD pipelines and external services can submit tasks
- **Separation of concerns**: Backend (orchestrator/agents) is decoupled from presentation

### Phase 2: Local Daemon with gRPC (Planned)

After the backend is stable, a local daemon will provide:
- **TUI**: Connects via gRPC over Unix Domain Socket (`/tmp/navi.sock`)
- **Security**: Protected by Unix file permissions
- **Performance**: Faster than TCP localhost

### Phase 3: Web UI (Planned)

Remote access via:
- **REST (grpc-gateway)**: CRUD operations
- **WebSocket**: Real-time LLM streaming

## REST API

The REST API provides HTTP endpoints for external integrations.

### Starting the Server

```bash
navi serve              # Start on :8080
navi serve --port 9000 # Start on :9000
```

The server also starts by default when running `navi` without arguments.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/agents` | List all agents |
| GET | `/agents/:id` | Get agent details |
| POST | `/tasks` | Create a new task |
| GET | `/tasks` | List all tasks |
| GET | `/tasks/:id` | Get task status |
| POST | `/agents/sync` | Sync agents from local agent roots into SQLite |

### API Examples

```bash
# Health check
curl http://localhost:8080/health

# List agents
curl http://localhost:8080/agents

# Create task
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Hello, world!"}'

# Get task status
curl http://localhost:8080/tasks/20260228123456-abc12345
```

### Task Request Body

```json
{
  "id": "optional-task-id",
  "agent_id": "optional-specific-agent",
  "prompt": "Task description"
}
```

### Task Response

```json
{
  "id": "task-id",
  "agent_id": "",
  "prompt": "Task description",
  "status": "pending|completed|failed",
  "output": "Result output",
  "error": "Error message if failed",
  "created_at": "2026-02-28T12:00:00Z"
}
```

## Configuration

All user configurations are stored in `~/.config/navi/` for full transparency and control:

| File | Purpose |
|------|---------|
| `~/.config/navi/config.toml` | Global settings (LLM provider, model, etc.) |
| `~/.config/navi/agents/<id>/` | Agent configurations |
| `~/.config/navi/workspace/<id>/` | Agent working directories |

### First Launch Behavior

On startup, Navi ensures the user config directory exists (`os.UserConfigDir()/navi`) before command execution.

### `.env` Support (Development)

Navi supports local `.env` loading for development:
- `.env`
- `.env.local`
- `.env.<NAVI_ENV>`
- `.env.<NAVI_ENV>.local`

If `NAVI_ENV` is empty, it defaults to `development`.

Useful vars:
- `NAVI_ENV`
- `NAVI_DEFAULT_PROVIDER`
- `NAVI_DEFAULT_MODEL`
- `NAVI_DEFAULT_API_KEY_ENV`
- `NAVI_API_KEY`
- `NAVI_LLM_BASE_URL`

In development mode, startup prints which env files were loaded.

### Example config.toml

```toml
[default_llm]
provider = "openai"
model = "gpt-4o-mini"
api_key_env = "OPENAI_API_KEY"
```

See `configs/config.example.toml` for more examples.

## Default Agents

Default agents are stored in `configs/agents/` in the repository:

```
configs/agents/
├── orchestrator/
├── coder/
└── researcher/
```

### Agent Validation System

Navi uses **filesystem as interface** and **SQLite as validator** (Checksum Store):

| File | Purpose |
|------|---------|
| `~/.config/navi/agents/<id>/` | Agent files (.toml, .md) |
| `~/.config/navi/agents.db` | SQLite: agent_id, path, file_hash, signature, status |

**Security:**
- **Trusted**: Agent validated, hash matches
- **Modified**: Manually edited, needs re-validation
- **Untrusted**: New agent without SQLite record, blocked until validated via SRP

**Detection:**
- fsnotify watches for file changes
- On boot: validates all agent hashes
- Manual edit detected → prompt: *"Agent X was modified. Authorize?"*

> Note: Validation/watch flow is architecture direction; parts are still being implemented incrementally.

## Sprint Status (Current)

### Sprint 1 (now)
- Simple Navi TUI / REPL ✅
- Basic main orchestrator agent ✅
- Basic tool calling for orchestrator ✅
- Basic MCP integration ✅
- `.env` local development support ✅
- Agent configuration folder requirements revision/re-think ✅
- Logging system foundation (`slog` + JSONL) ✅

**Sprint 1 goal:** Run TUI, ask model to use tools, and verify tools execute correctly.

### Sprint 2 (planned)
- Basic specialist agents (planner, researcher, coder, tester), one active at a time
- Basic native MCP tools expansion
- More CLI commands
- TUI UX improvements

**Sprint 2 goal:** In TUI chat, model can call MCP tools and delegate to specialist agents (without full agency yet).

### Sprint 3 (planned)
- Agency behavior and multi-agent coordination

## Dependencies

- `github.com/go-chi/chi/v5` - HTTP router
- `github.com/BurntSushi/toml` - Config parsing
- `github.com/glebarez/sqlite` - Database
- `github.com/joho/godotenv` - `.env` support for development
- `google.golang.org/grpc` - gRPC (planned)

## Common Commands

```bash
go build ./...          # Build
go test ./...           # Run tests
go run ./cmd/navi/main.go serve  # Run API server
go run ./cmd/navi/main.go repl   # Run REPL
go run ./cmd/navi/main.go chat "hello"  # Single chat message
```
