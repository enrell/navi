# Navi - A Secure AI Orchestrator Built for the Community

> POC - Will change a lot, contact-me to suggest something.

<p align="center">
  <em>NAVI: Your gateway to the Wired</em>
</p>

<p align="center">
  <a href="https://go.dev/">
    <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go" alt="Go Version">
  </a>
  <a href="https://github.com/enrell/navi/blob/main/LICENSE">
    <img src="https://img.shields.io/badge/License-MIT-green.svg?style=for-the-badge" alt="License">
  </a>
  <a href="#status">
    <img src="https://img.shields.io/badge/Status-Prototype-orange?style=for-the-badge" alt="Status">
  </a>
  <a href="https://discord.gg/eNsMFGZU">
    <img src="https://img.shields.io/badge/Discord-Join-5865F2?style=for-the-badge&logo=discord" alt="Discord">
  </a>
</p>

---

## 🚨 Project Status: Early Development

**Navi is currently a prototype.** The core architecture is being designed, and no production-ready code has been released yet. This repository will be updated as development progresses.

> **Why this exists:** Most AI orchestrators are built as products to sell subscriptions, not as open-source tools for the community. Navi is different—it's built **for developers, by developers**, with security and real utility as non-negotiable principles.

---

## What is Navi?

Navi is an AI orchestrator designed to be the **maestro of your automation orchestra**. It connects disparate tools, scripts, and services without requiring ugly glue code, while keeping **you in full control**.

Unlike "autonomous agents" that might hallucinate and run `rm -rf /` on your production server, Navi follows a golden rule: **you are always in control, and security is non-negotiable**.

The name comes from **NAVI**, the computer from the anime [*Serial Experiments Lain* (1998)](https://anilist.co/anime/339/serial-experiments-lain/), which the protagonist uses to access "the Wired"—a global network mixing virtual reality, collective consciousness, and more.

---

## Why Navi Exists

### The Problem with Current AI Orchestrators

After testing tools like OpenClaw and others, a pattern emerged:

- 🚫 Built as **products to be sold**, not open-source community tools
- 🚫 Push generic "agency" with bloated features that aren't actually useful
- 🚫 Designed for marketing hype and subscription sales
- 🚫 Security is an afterthought (open ports, plaintext credentials, 1-click RCE)
- 🚫 Acqui-hired by big tech, then fade into irrelevance

### The Navi Difference

| Traditional Orchestrators | Navi |
|---------------------------|------|
| Subscription-focused | Community-focused |
| Bloated feature sets | Solves real problems |
| Security as afterthought | Security is non-negotiable |
| Lock-in via proprietary UI | Multiple open interfaces |
| "Magical autonomous agents" | Human-in-the-loop by design |
| One-size-fits-all | User-configurable isolation |

---

## Goals & Threat Model

### Goals

- **Build real autonomous agents** for development, automation, and desktop workflows
- **Prevent accidental OS damage** caused by LLM hallucinations
- **Keep every side effect auditable and reversible**
- **Support GUI workflows** without breaking isolation guarantees
- **Remain cross-platform** with flexible isolation backends
- **User choice** in isolation strategy (Docker, Bubblewrap, native)

### Explicit Threat Model

#### In Scope (Mitigated)
- ✅ Accidental filesystem destruction
- ✅ Unintended network exposure
- ✅ Workspace corruption
- ✅ Silent privilege escalation

#### Out of Scope (Accepted)
- ❌ Kernel zero-days
- ❌ Compromised host
- ❌ Malicious user intent

> **This system protects against agent mistakes, not hostile adversaries.**

---

## Core Architectural Principles

### 1. Hexagonal Architecture (Ports & Adapters)

Navi is built on **Hexagonal Architecture**, which means:

- **Core logic is isolated** from external concerns (isolation backends, LLM providers, UI modes)
- **Swappable implementations** without changing business logic
- **Testable interfaces** without calling actual models or services
- **User choice** in every layer

```
┌─────────────────────────────────────────────────────────────┐
│                    Entry Points (Ports)                      │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │   TUI    │  │ REST API │  │ Discord  │  │ Telegram │    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────┐
│                 Authentication Layer (Required)               │
│  - API Keys / Tokens                                        │
│  - User Sessions                                            │
│  - Permission Checks                                        │
└──────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────┐
│                  Orchestrator (Core Logic)                   │
│  ┌────────────────────────────────────────────────────────┐  │
│  │           Agency Coordination Layer                    │  │
│  │  - Planner Agent   - Researcher Agent                  │  │
│  │  - Coder Agent     - Verifier Agent                    │  │
│  │  - Executor Agent  - Custom Agents                     │  │
│  └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────┐
│                    Adapters (User-Selectable)                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│  │ Isolation    │  │ LLM Provider │  │ Storage      │       │
│  │ - Docker     │  │ - OpenAI     │  │ - SQLite     │       │
│  │ - Bubblewrap │  │ - Anthropic  │  │ - Postgres   │       │
│  │ - Native     │  │ - Ollama     │  │ - S3         │       │
│  └──────────────┘  └──────────────┘  └──────────────┘       │
└──────────────────────────────────────────────────────────────┘
```

### 2. User-Configurable Isolation

**Docker is optional.** Navi supports multiple isolation backends:

| Isolation Backend | Best For | Pros | Cons |
|-------------------|----------|------|------|
| **Docker** | VPS, multi-user, production | Strong isolation, cross-platform, resource limits | Heavier, requires daemon |
| **Bubblewrap** | Linux desktop | Lightweight, no daemon, fast setup | Linux-only |
| **Native Restricted** | Simple tasks, trusted environments | Minimal overhead, no dependencies | Weaker isolation |

Users choose their isolation strategy based on their use case:
- **VPS / Server**: Docker for strong multi-tenant isolation
- **Linux Desktop**: Bubblewrap for lightweight local automation
- **Trusted Local**: Native sandbox for simple tasks

### 3. Capability-Based Authority

Authority is expressed as **explicit capabilities**, enforced by the chosen isolation backend:

- ❌ No implicit global filesystem access
- ❌ No unrestricted host execution
- ❌ No automatic network exposure

**Capability Configuration Example:**
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

**Workspace Mount:**
```bash
# Only default writable path
-v /host/workspace:/workspace

# Additional mounts require explicit user approval
```

### 4. Mandatory Authentication

**All interaction modes require authentication.** No exceptions.

| Mode | Authentication Methods |
|------|------------------------|
| **REST API** | API keys, JWT tokens, OAuth |
| **REPL** | Local token or password |
| **Discord Bot** | Discord OAuth + user linking |
| **Telegram Bot** | Telegram user ID + PIN |

---

## Development Strategy

Navi is being developed in phases to ensure a solid foundation:

### Phase 1: REST API First (Current)
The REST API is the primary interface during initial development. This approach:
- **Facilitates rapid development**: External tools and scripts can easily integrate
- **Simplifies debugging**: HTTP is easier to inspect than terminal UI
- **Enables automation**: CI/CD pipelines and external services can submit tasks
- **Separation of concerns**: Backend (orchestrator/agents) is decoupled from presentation

### Phase 2: TUI (Planned)
A Terminal User Interface will be added once the backend is stable.

---

## Interaction Modes

Navi doesn't lock you into a single UI. Choose your interface:

### 1. REST API (Default)
- Full HTTP API for mobile apps and integrations
- **Start**: `navi serve` or just `navi`
- **Authentication**: API keys, JWT

### 2. REPL (Terminal)
- Interactive terminal REPL
- **Start**: `navi repl`
- **Authentication**: local token or password

### 3. Messaging Bots
- **Discord**: Secure server automation
- **Telegram**: Remote task triggering
- **Authentication**: user linking + optional PIN

### 4. Custom Modes
Define your own interaction mode via the adapter pattern.

---

## Operation Modes

Navi supports multiple operation modes, configurable by the user:

### 1. VibeCode Mode
For rapid prototyping and experimentation:
- Quick iteration with minimal friction
- Temporary sandboxes
- Ideal for learning and testing

### 2. Production Mode
For critical workflows:
- Strict capability enforcement
- Multi-agent verification
- Human-in-the-loop approvals
- Full audit trail

### 3. Custom Modes (User-Defined)
Users can define custom modes via LLM-generated configurations:

```yaml
# Example: Daily AI News Mode
mode:
  name: "daily-news"
  trigger: "cron: 0 12 * * *"  # Every day at 12 AM
  agents:
    - Researcher: "Fetch AI news from configured sources"
    - Summarizer: "Summarize top 5 stories"
    - Notifier: "Send to Telegram/Discord"
  isolation: docker
  approval: false  # Auto-execute for this mode
  output:
    - telegram: "channel_id"
    - discord: "channel_id"
```

Users can ask the LLM to generate custom modes based on their needs:
> "Create a mode that monitors my GitHub PRs every hour and posts a summary to Discord"

---

## Intelligent Parallelism & Agent Delegation

Navi implements **smart parallelism** to give real agency to agents. Agents can **delegate tasks among themselves** based on capability, workload, and context.

### How Delegation Works

```
┌──────────────┐
│ Main Agent   │ Receives task: "Build a REST API with auth"
└──────┬───────┘
       │
       ├──► Delegates to Planner Agent
       │    "Break down the task"
       │
       ├──► Planner creates subtasks:
       │    1. Research auth patterns (Researcher)
       │    2. Design API structure (Planner)
       │    3. Implement endpoints (Coder)
       │    4. Write tests (Coder + Verifier)
       │    5. Build & run (Executor)
       │
       ├──► Parallel Execution:
       │    ┌───────────────┐  ┌───────────────┐
       │    │ Researcher    │  │ Planner       │
       │    │ (searches     │  │ (designs      │
       │    │  patterns)    │  │  structure)   │
       │    └───────────────┘  └───────────────┘
       │             │                │
       │             └──────┬─────────┘
       │                    ▼
       │             ┌───────────────┐
       │             │ Coder         │
       │             │ (implements)  │
       │             └───────────────┘
       │                    │
       │                    ▼
       │             ┌───────────────┐
       │             │ Verifier      │
       │             │ (validates)   │
       │             └───────────────┘
       │
       └──► Orchestrator collects results and presents to user
```

### Parallelism Benefits

| Single Thread | Intelligent Parallelism |
|---------------|------------------------|
| Sequential execution | Concurrent task processing |
| One agent bottleneck | Distributed workload |
| Linear time cost | Optimized completion time |
| No task delegation | Dynamic task routing |

---

## Observability & Audit

**No hidden side effects.** Every action is logged and traceable:

- ✅ **Every task generates an event**
- ✅ **Every container/process start/stop logged**
- ✅ **Every capability grant logged**
- ✅ **Every filesystem mutation tied to commit hash**

### Event Log Structure (SQLite WAL)

```sql
CREATE TABLE event_log (
  id INTEGER PRIMARY KEY,
  timestamp TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  action TEXT NOT NULL,
  capability TEXT,
  workspace_path TEXT,
  git_commit TEXT,
  result TEXT,
  error TEXT,
  user_id TEXT NOT NULL
);
```

---

## Understanding LLM Limitations

Navi is built around LLM weaknesses, not their strengths:

| Weakness | How Navi Addresses It |
|----------|----------------------|
| **Context Dilution** | Specialized agents with focused contexts |
| **Overconfidence** | Verifier agent + human-in-the-loop |
| **No Memory** | External persistence with SQLite + vectors |
| **Generalist Trap** | Domain-specific agents |
| **Hallucinations** | Multi-agent verification + sandboxed execution |
| **Accidental Damage** | Isolation backend + capability-based authority |

---

## Tech Stack

- **Language**: Go 1.21+
- **Architecture**: Hexagonal (Ports & Adapters)
- **Isolation**: Docker, Bubblewrap, or Native (user choice)
- **UI**: REST API (default), REPL, Bubble Tea TUI (planned)
- **Database**: SQLite (pure Go, no CGO) with WAL mode
- **API**: Chi router for HTTP
- **Authentication**: JWT, API Keys, OAuth adapters
- **LLM Clients**: OpenAI-compatible API pattern
- **MCP**: Model Context Protocol support

---

## Getting Involved

This is an **open-source project** focused on solving real problems without selling out to the hype cycle.

### How to Contribute

1. **Watch this space**: Code will be pushed soon
2. **Join the discussion**: [Discord](https://discord.gg/eNsMFGZU)
3. **Follow updates**: [X/Twitter](https://x.com/enrellsan)
4. **Star the repo**: Show support while we build

### What You Can Expect

- Public development (messy experimentation included)
- Focus on security and real utility
- No subscription traps or vendor lock-in
- Community-driven feature priorities
- User choice in every layer

---

## License

MIT License - See [LICENSE](LICENSE) file for details.

---

## Acknowledgments

- **Lain Iwakura**: For inspiring the name and vision
- **Serial Experiments Lain**: For being dense, intellectual, and philosophical
- The open-source Go community for incredible tooling
- Everyone building real tools instead of hype

---

> *"Present day, present time!"*
>
> Infrastructure survives bubbles. Hype doesn't. Build the former.

---

<p align="center">
  <sub>Built with ❤️ by <a href="https://github.com/enrell">@enrell</a></sub>
</p>
