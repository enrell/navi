# AGENTS.md - Guide for Working in the Navi Repository

## Project Overview

**Navi** is an AI orchestrator built in Go with a hexagonal (ports & adapters) architecture. It provides secure, controllable AI agent execution with multiple isolation backends (Docker, Bubblewrap, native).

- **Status**: Early prototype/POC - expect breaking changes
- **Language**: Go 1.25+
- **License**: MIT

## Directory Structure

Current state:

```
navi/
├── cmd/
│   └── cli/              # CLI entry point (Cobra) - main.go currently empty
│       └── main.go
├── internal/             # Private application code - NOT YET CREATED
│   ├── domain/           # Domain interfaces (ports)
│   ├── adapters/         # External adapters (OpenAI, Docker, etc.)
│   ├── orchestrator/     # Core coordination logic
│   ├── agents/           # Agent implementations
│   └── tui/              # Terminal UI
├── configs/              # Configuration files (empty)
├── tests/                # Test files (empty)
├── docs/                 # Extensive documentation
│   ├── architecture/overview.md
│   ├── components/agents.md
│   ├── components/isolation-adapters.md
│   ├── getting-started.md
│   ├── interfaces/index.md
│   ├── security/model.md
│   └── index.md
├── go.mod               # Go module definition (minimal)
├── go.sum               # Go module checksums
├── README.md            # Project vision and roadmap
├── CONTRIBUTING.md      # Contribution guidelines
└── LICENSE              # MIT license
```

Planned `internal/` structure (from docs):

```
internal/
├── domain/                  # Ports (interfaces)
│   ├── provider.go          # LLMPort
│   ├── isolation.go         # IsolationPort
│   ├── repository.go        # RepositoryPort
│   └── auth.go              # AuthPort
├── adapters/                # Adapter implementations
│   ├── openai_adapter.go
│   ├── anthropic_adapter.go
│   ├── docker_adapter.go
│   ├── bubblewrap_adapter.go
│   └── native_adapter.go
├── orchestrator/            # Core orchestrator
│   ├── orchestrator.go
│   ├── factory.go
│   └── agency.go
├── agents/                  # Agent implementations
│   ├── planner/
│   ├── researcher/
│   ├── coder/
│   ├── executor/
│   ├── verifier/
│   └── prompts/             # Text prompt files
├── tui/                     # Terminal UI (Bubble Tea)
│   └── tui.go
├── api/                     # REST API (Chi)
└── storage/                 # Database layer (SQLite)
```

## Essential Commands

### Build

```bash
# Build all packages
go build ./...

# Build the CLI binary
go build -o navi ./cmd/cli

# Install to GOPATH/bin
go install ./cmd/cli
```

### Test

```bash
# Run all tests
go test ./...

# Verbose
go test -v ./...

# With race detection
go test -race ./...

# Integration tests (when they exist)
go test -tags=integration ./...

# E2E tests (requires Docker/bwrap)
go test -tags=e2e ./...
```

### Run

```bash
# Run the CLI (once implemented)
go run ./cmd/cli

# After building:
./navi

# TUI mode (once implemented)
go run ./cmd/cli tui
./navi tui

# Start REST API server (once implemented)
./navi serve --port 8080
```

### Format & Lint

```bash
# Format code
go fmt ./...

# Vet for common issues
go vet ./...

# Lint (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
golangci-lint run
```

### Dependency Management

```bash
# Download dependencies
go mod download

# Tidy go.mod/go.sum
go mod tidy

# Verify dependencies
go mod verify

# Add a dependency
go get github.com/some/module
go mod tidy
```

## Code Organization & Architecture

### Hexagonal Architecture (Ports & Adapters)

Navi follows **hexagonal architecture**. Core logic depends only on interfaces (ports); adapters implement ports for external services.

```
┌─────────────────────────────────────────────┐
│                  ADAPTERS                   │
│  (OpenAIAdapter, DockerAdapter, SQLiteRepo) │
├─────────────────────────────────────────────┤
│                CORE LOGIC                   │
│        Depends only on:                     │
│         - LLMPort                           │
│         - IsolationPort                     │
│         - RepositoryPort                    │
├─────────────────────────────────────────────┤
│                    PORTS                    │
│ (Interfaces: LLMPort, IsolationPort, etc.)  │
└─────────────────────────────────────────────┘
```

Keep `internal/domain/` pure - no adapter-specific code. Adapters in `internal/adapters/` implement domain interfaces.

### Core Components (Planned)

1. **Orchestrator**: Coordinates agents, enforces security, audits.
2. **Agency**: Multi-agent system:
   - Planner (decomposition)
   - Researcher (info gathering)
   - Coder (code generation)
   - Executor (tool execution)
   - Verifier (validation)
3. **Adapters**: LLM providers, isolation backends, storage, auth.
4. **Entry Points**: CLI, TUI (Bubble Tea), REST API (Chi), bots.

### Data Flow (Intended)

User → CLI/TUI/API → Auth → Orchestrator → Planner → [parallel agents] → Executor → Verifier → Result → Audit log.

Every operation: authenticate, check capabilities, sandboxed execution, log event.

## Naming Conventions

From `CONTRIBUTING.md` and Go idioms:

- Packages: lowercase, short, descriptive.
- Exported: `CamelCase`; unexported: `camelCase`.
- Interfaces: behavior names (`Reader`, `Writer`); `-er` suffix common.
- Constructors: `New{Thing}(...)`.
- Files: lowercase (`openai_adapter.go`).
- Comments: exported items need full sentences, explain why.

### Error Handling

- Return `error`; avoid `panic()` in production.
- Wrap with `%w`: `return fmt.Errorf("failed: %w", err)`.
- Never swallow errors silently.

### Context Usage

- Accept `context.Context` as first param.
- Pass through all downstream calls.
- Respect cancellation and deadlines.

## Testing Strategy

From `CONTRIBUTING.md`:

- Table-driven tests.
- Mock dependencies via interfaces.
- Use `testing.Short()` to skip integration tests.
- Add benchmarks when relevant.

Currently **no tests exist**. Add tests as features are built.

## Important Gotchas & Non-Obvious Patterns

### 1. Early Development State

- **POC**: Minimal code, many placeholders.
- **Breaking changes**: Expect APIs to evolve.
- **Planned directories missing**: `internal/` not yet created.
- **Dependencies**: `go.mod` has no third-party deps yet; will be added incrementally.

### 2. Security-First Mindset

- **Security-sensitive**: AI execution with user data.
- **Capability-based authority**: All operations must be explicitly granted.
- **No secrets in source**: Use env vars or config with `0600`.
- **Audit everything**: Every action logs an event.

### 3. Hexagonal Discipline

- Core (`domain`) depends only on interfaces.
- Adapters implement interfaces; no core code in adapters.
- Entry points depend on core, not directly on adapters.

### 4. Capability Model

- Explicit grants: filesystem paths, network hosts, exec binaries.
- No implicit global access.
- Adapters translate capabilities to backend config (Docker, bwrap, etc.).

### 5. Agent Communication

Messages via orchestrator:

```go
type AgentMessage struct {
    From    string
    To      string
    Type    string // "request", "response", "event", "error"
    Payload interface{}
}
```

All messages persisted to event log.

### 6. Isolation Backends

| Backend   | Security | Performance | Platform    |
|-----------|----------|-------------|-------------|
| Docker    | High     | Medium      | Cross       |
| Bubblewrap| High     | High        | Linux only  |
| Native    | Medium   | Highest     | All         |

Adapters enforce capabilities and provide `Execute`, `FileRead`, `FileWrite`.

### 7. Configuration

Config file locations (in order):
1. `./navi.yaml`
2. `~/.config/navi/config.yaml`
3. `/etc/navi/config.yaml`

Key env vars:
- `NAVI_OPENAI_API_KEY`
- `NAVI_ANTHROPIC_API_KEY`
- `NAVI_DB_PATH`
- `NAVI_LOG_LEVEL`
- `NAVI_CONFIG`

### 8. Event Sourcing

All actions logged to SQLite (WAL). Schema:

```sql
CREATE TABLE event_log (
    id TEXT PRIMARY KEY,
    timestamp TEXT NOT NULL,
    user_id TEXT NOT NULL,
    agent_id TEXT,
    action TEXT NOT NULL,
    resource TEXT,
    details TEXT,
    result TEXT NOT NULL,
    error TEXT,
    git_commit TEXT,
    INDEX idx_timestamp (timestamp),
    INDEX idx_user_id (user_id),
    INDEX idx_action (action)
);
```

### 9. Git Integration

Workspaces are git-tracked; changes auto-committed (when implemented). Enables rollback and audit.

### 10. Parallelism

Orchestrator runs independent agent tasks concurrently via goroutines, respecting dependencies.

## Project-Specific Context from Docs

Read the `docs/` files for detailed design:

- `architecture/overview.md` - Hexagonal architecture, data flow, boundaries.
- `components/agents.md` - Agent types, communication, lifecycle, prompts.
- `components/isolation-adapters.md` - Backend configs, seccomp, performance.
- `interfaces/index.md` - TUI, REST API, Discord, Telegram specs.
- `security/model.md` - Threat model, capabilities, auth.
- `getting-started.md` - Setup, config, run, debug.

These are design documents; implementation may vary but should align.

## Dependencies

Current `go.mod`:

```go
module navi
go 1.25.0
```

No third-party deps yet. As features are built, add via `go get` and `go mod tidy`.

Expected future dependencies:
- `github.com/spf13/cobra` (CLI)
- `charm.land/bubbletea/v2` (TUI)
- `github.com/go-chi/chi` (HTTP)
- SQLite driver (`modernc.org/sqlite` or `mattn/go-sqlite3`)
- `github.com/golang-jwt/jwt/v5` (JWT)
- LLM provider SDKs.

## Tools

- **gopls**: `go install golang.org/x/tools/gopls@latest`
- **golangci-lint**: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
- **goimports**: `go install golang.org/x/tools/cmd/goimports@latest`

Configure your editor to run `go fmt`/`goimports` on save.

## Workflow (from CONTRIBUTING.md)

1. Discuss changes in an issue first.
2. Branch: `git checkout -b feat/description` (or `fix/`, `docs/`, `refactor/`, `test/`).
3. Make small, focused changes with tests and documentation.
4. Run: `go fmt ./...`, `go vet ./...`, `go test ./...`, `golangci-lint run`.
5. Commit using [Conventional Commits](https://www.conventionalcommits.org/):
   ```
   feat(isolation): add Firecracker microVM adapter

   - Implement FirecrackerAdapter with configurable VM size
   - Add seccomp filter for syscall restriction

   Closes #123
   ```
6. Push and open PR with clear description, screenshots for UI, testing steps.

## Security Checklist

When writing code:
- [ ] All user input validated.
- [ ] All operations capability-checked.
- [ ] All external calls authenticated.
- [ ] No secrets logged.
- [ ] SQL queries parameterized.
- [ ] Error messages don't leak data.
- [ ] No `panic()` in production code.

## Memory Files

This section lists useful commands and conventions for agents (like you) working in this repo:

- Build: `go build ./...`
- Test: `go test ./...`
- Format: `go fmt ./...`
- Vet: `go vet ./...`
- Run CLI: `go run ./cmd/cli`
- Run TUI: `go run ./cmd/cli tui` (once implemented)
- Download deps: `go mod download`
- Tidy deps: `go mod tidy`
- Lint: `golangci-lint run`

## References

- `README.md` - Vision, principles, roadmap.
- `CONTRIBUTING.md` - Contribution guidelines, coding standards, testing.
- `docs/architecture/overview.md` - Hexagonal architecture details.
- `docs/security/model.md` - Full security model.
- `docs/getting-started.md` - Setup, configuration, troubleshooting.
- `docs/components/agents.md` - Multi-agent system design.
- `docs/components/isolation-adapters.md` - Isolation backend specs.

## Notes

- This is a **community-driven, security-first** project. Build real utility, not hype.
- The codebase is **early POC**. Much of the documented design is not yet implemented.
- **Do not** introduce new architectural patterns without discussion. Follow hexagonal architecture as defined.
- **Always** enforce capabilities; assume agents can malfunction or be malicious.
- **Keep core pure**: No adapter-specific types in `internal/domain/`.
- When in doubt, read the docs in `docs/` and ask in Discord (link in README).
