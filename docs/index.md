# Navi Documentation

Welcome to the official documentation for **Navi**, a secure AI orchestrator built for the developer community.

## What is Navi?

Navi is an open-source AI orchestrator that connects disparate tools, scripts, and services without glue code, while keeping you in full control. It's designed to be the **maestro of your automation orchestra**.

**Key Differentiators:**
- **Security-first**: Capability-based authority, mandatory authentication, isolation backends
- **Multi-agent coordination**: Specialized agents work together, not a single "god agent"
- **Hexagonal architecture**: Swap providers without changing business logic
- **Community-driven**: Built for utility, not to sell subscriptions

## Quick Links

- [Getting Started](getting-started.md) - Build and run Navi
- [Architecture](architecture/overview.md) - Deep dive into the design
- [Security Model](security/model.md) - How Navi keeps you safe
- [Components Reference](../README.md#core-architectural-principles) - Detailed component docs

## Core Concepts

### Hexagonal Architecture (Ports & Adapters)

```
┌─────────────────────────────────────────────┐
│                 ADAPTERS                    │
│   (LLM Provider, Database, OS Scripts)     │
├─────────────────────────────────────────────┤
│              CORE LOGIC                     │
│   (Orchestration Intelligence)              │
│           ↓ ISOLATED ↓                      │
├─────────────────────────────────────────────┤
│                  PORTS                      │
│   (Entry Points / Interfaces)               │
└─────────────────────────────────────────────┘
```

Business logic is completely isolated from external concerns. This means:
- Swappable LLM providers (OpenAI, Anthropic, Ollama)
- Swappable isolation backends (Docker, Bubblewrap, Native)
- Swappable storage (SQLite, Postgres, S3)
- Swappable UIs (TUI, REST API, Discord, Telegram)

### Runtime Engine — Agents Are Data

Navi uses a **GenericAgent Runtime Engine**. Rather than hardcoding agent types in Go, every agent is defined by two files:

```
~/.config/navi/agents/<name>/
├── config.toml   ← LLM, capabilities, isolation backend
└── AGENT.md      ← system prompt (the agent's "brain")
```

Create a new specialist (researcher, coder, planner, security auditor…) by writing config files — **no recompilation**. The orchestrator hot-loads new agents at runtime via `navi agent create`.

Benefits:
- **Zero-downtime** agent creation and removal
- **Safer** than plugin marketplaces — no untrusted Go code, just config files
- **LLM-generatable** — ask Navi to design a new agent for you
- **Composable** — route tasks to agents by their capability set

### Default Agents

Default agents are stored in `configs/agents/` in the repository. During installation, they are copied to `~/.config/navi/agents/`.

#### The Navi Solution: Hybrid Storage Model

Navi uses **filesystem as the interface** and **SQLite as the validator** (Checksum Store).

**Storage**: Agents in `~/.config/navi/agents/` as `.toml` and `.md` files.

**Validation**: SQLite contains:
- `agent_id`, `path`, `file_hash`, `signature`, `status`

**Security Features**:
- **Manual Edit Detection**: If you edit an agent file manually (Neovim, etc.), Navi detects via fsnotify and prompts: *"Agent X was modified. Authorize with your key?"*
- **Injection Blocking**: New agents without SQLite record are marked **Untrusted** and won't load until validated via SRP

To update: Navi prompts you on startup or call `POST /agents/sync`.

### Capability-Based Authority

Navi doesn't give blanket access. Every operation is granted explicit capabilities:

```json
{
  "agent": "coding",
  "capabilities": {
    "filesystem": ["workspace:rw", "/home/user/project:ro"],
    "network": ["api.github.com"],
    "ports": [],
    "exec": ["bash", "node", "go"]
  }
}
```

This prevents accidental damage from LLM hallucinations.

## Entry Points & Communication

Navi supports multiple entry points, each using the appropriate communication protocol:

| Entry Point | Communication | Security |
|-------------|---------------|----------|
| **TUI** (local) | gRPC via Unix Domain Socket (`/tmp/navi.sock`) | Unix file permissions |
| **Web UI** (remote) | REST (grpc-gateway) + WebSocket | SRP + Token + HTTPS |
| **API/Bots** (remote) | REST | API Keys / PASETO + HTTPS |
| **Desktop App** | gRPC or REST (local) | Token saved in OS Keyring |

### Why Unix Domain Sockets?

For local communication (TUI, Desktop App), Navi uses **Unix Domain Sockets** instead of TCP:

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

## Authentication

### Local Access (TUI)

- **Unix Domain Socket**: Protected by file permissions
- No additional auth required if user has socket access

### Remote Access (Web UI, API)

| Mode | Authentication |
|------|----------------|
| Web UI | SRP (Secure Remote Password) + Token Opaco (Cookie HttpOnly) |
| API / Bots | API Keys (hashed) or PASETO |
| Desktop App | Token stored in OS Keyring |

## Isolation Backends

Choose your isolation strategy:

| Backend | Best For | Pros | Cons |
|---------|----------|------|------|
| **Docker** | VPS, multi-user, production | Strong isolation, cross-platform | Heavier, requires daemon |
| **Bubblewrap** | Linux desktop | Lightweight, no daemon | Linux-only |
| **Native Restricted** | Simple tasks, trusted | Minimal overhead | Weaker isolation |

## Agent Sync System

1. **Installation**: Setup script pulls default agents from GitHub
2. **Startup**: Navi checks for updates
3. **Prompt**: "Update agents from GitHub? (y/N)"
4. **Sync**: API endpoint `POST /agents/sync` pulls latest changes
5. **Hot-reload**: Agents updated without restart

## Project Status

**Navi is in early development**.

See the [full roadmap](../README.md#roadmap) for details.

## Philosophy

> *"Infrastructure survives bubbles. Hype doesn't. Build the former."*

Navi is built with the belief that:
- Tools should be **useful**, not just hyped
- Security is **non-negotiable**, not an afterthought
- Users should have **choice** in every layer
- The community should **own** the tools, not rent them

## Get Involved

- **GitHub**: https://github.com/enrell/navi
- **Discord**: https://discord.gg/eNsMFGZU
- **License**: MIT
- **Contributing**: See [CONTRIBUTING.md](../CONTRIBUTING.md)

---

*Built with ❤️ by the Navi community*
