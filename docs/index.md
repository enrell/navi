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

### Multi-Agent Agency

Instead of a single monolithic agent that tries to do everything, Navi uses a **specialized agency** pattern:

```go
type Agency struct {
    Planner    *Agent  // Decides the steps
    Researcher *Agent  // Searches and gathers info
    Coder      *Agent  // Writes and reviews code
    Executor   *Agent  // Runs tools and APIs
    Verifier   *Agent  // Validates results
}
```

Each agent has a **single responsibility**, leading to:
- Reduced context dilution
- Lower hallucination rates
- Clear failure boundaries
- Easier debugging

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
