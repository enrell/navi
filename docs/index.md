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
│   (LLM Provider, Database, OS Scripts)      │
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
- swappable UIs (TUI, REST API, Discord, Telegram)

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

## Interaction Modes

Navi supports multiple entry points:

| Mode | Description | Authentication |
|------|-------------|----------------|
| **TUI** | Terminal UI with Bubble Tea | Local token, biometric, password |
| **REST API** | HTTP API for integrations | API keys, JWT, OAuth |
| **Discord Bot** | Secure server automation | Discord OAuth + user linking |
| **Telegram Bot** | Remote task triggering | Telegram user ID + PIN |

All modes require authentication. No exceptions.

## Isolation Backends

Choose your isolation strategy:

| Backend | Best For | Pros | Cons |
|---------|----------|------|------|
| **Docker** | VPS, multi-user, production | Strong isolation, cross-platform | Heavier, requires daemon |
| **Bubblewrap** | Linux desktop | Lightweight, no daemon | Linux-only |
| **Native Restricted** | Simple tasks, trusted | Minimal overhead | Weaker isolation |

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
- **Contributing**: See [CONTRIBUTING.md](../CONTRIBUTING.md) (coming soon)

---

*Built with ❤️ by the Navi community*
