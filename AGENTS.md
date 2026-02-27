# Agents Architecture

This document describes Navi's multi-agent system from an architectural perspective: how agents are defined, how they interact with the orchestrator, and the security model that governs them.

## Core Principle: Configuration-Driven Agents

Unlike traditional systems where agent types are hard-coded, Navi treats **agents as data**. An agent is defined by two files:

- `config.toml` — declares capabilities, isolation backend, LLM settings
- `AGENT.md` — system prompt that shapes the LLM's behavior

The runtime provides a single `GenericAgent` implementation that reads these files and operates accordingly. This enables users to create, share, and modify agents without recompiling Navi.

## Agent Identity & Trust

### Registration

Agents become **trusted** only after going through the **authenticated creation flow**:

1. User initiates agent creation via the TUI
2. TUI prompts for authentication (password/biometric)
3. Upon successful auth, TUI sends a `create_agent` tool call to the orchestrator
4. Orchestrator writes the configuration files to `~/.config/navi/agents/<name>/`
5. Orchestrator records the agent in the `agent_registry` table with `is_trusted = true`
6. Orchestrator emits an `agent.created` event; TUI updates UI

Only agents in the registry with `is_trusted = true` are allowed to invoke privileged tools (file write, shell exec, create_agent, etc.).

### Untrusted Agents

Any tool call bearing an `agent_id` that:

- Is not present in the `agent_registry`, OR
- Has `is_trusted = false`

is rejected immediately and logged as `untrusted_agent` or `unregistered_agent`. The TUI may show an alert, allowing the user to register the agent if legitimate.

This prevents:
- Malicious external scripts from calling internal tools directly
- Accidentally using an agent that was copied from elsewhere without registration
- Unauthorized privilege escalation

## Capability Model

Agents operate under **capability-based authority**. The `config.toml` includes a `capabilities` list that grants specific permissions:

- `filesystem:/path:rw` — read/write access to a path
- `network:host:port` — network access to host:port
- `exec:binary1,binary2` — allowed executables
- `vision`, `ocr`, `audio` — multimodal capabilities

These capabilities are **deny-by-default**. The orchestrator checks an agent's capabilities before:
- Assigning a task (does the task require capabilities the agent lacks?)
- Allowing a tool call (does the tool require a capability the agent lacks?)

Isolation adapters enforce capabilities at runtime (e.g., Docker volume mounts, seccomp filters).

## Tool Calls & Authentication

Certain operations are exposed as **tools** that agents can invoke. Tools are not directly reachable by agents; they must be called via a structured `ToolCall` message to the orchestrator.

### Tool Categories

1. **Unprivileged tools** — no user interaction needed (e.g., `file_read`, `http_get` if capability granted)
2. **Privileged tools** — require user authentication before execution (e.g., `create_agent`, `file_write`, `shell_exec`)

### Auth Flow for Privileged Tools

```
Agent → Orchestrator: ToolCall{tool:"create_agent", args:{...}, request_id:"uuid"}

Orchestrator:
  - Verify agent is trusted (in registry)
  - Check agent has capability for this tool
  - Tool.RequiresAuth() == true → emit AuthRequest event with RequestID and description
  → Block and wait

TUI receives AuthRequest event → shows modal:
  "Agent 'orchestrator' wants to: Create new agent 'research-2'"
  [Authenticate] [Cancel]

User enters password → TUI sends AuthResponse{RequestID, Credentials}

Orchestrator:
  - Verify credentials (against system or stored hash)
  - If valid → proceed with Tool.Execute()
  - If invalid → reject, log `auth_failed`
  - Respond to agent with ToolResponse or Error
```

### Replay Protection

Every tool call includes a unique `request_id` (UUID v4). The orchestrator tracks recent IDs to prevent replay attacks. Once a `request_id` is used, it cannot be reused.

## Agent Communication

Agents do not communicate directly with each other. All messages go through the orchestrator:

- Incoming: `AgentMessage{From:"agent-x", To:"agent-y", Type:"request", Payload:Task}`
- Orchestrator validates that `agent-x` and `agent-y` exist and are trusted, then routes.

All messages are persisted to the event log for audit.

## Lifecycle

1. **Startup** — Orchestrator scans `~/.config/navi/agents/*/config.toml`, instantiates `GenericAgent` for each, starts message loop goroutine
2. **Task Assignment** — Orchestrator selects idle agent with required capabilities, sends `AgentMessage`
3. **Execution** — Agent builds prompt from system + task, calls LLM, may invoke tools via orchestrator
4. **Shutdown** — Context cancellation; all agent goroutines exit gracefully

## Security Boundaries

- **Capability boundary** — Agents cannot exceed declared capabilities; enforced by orchestrator and isolation adapters
- **Authentication boundary** — Privileged tools require user presence; no passwordless elevation
- **Registration boundary** — Only orchestrator can register agents, and only via authenticated TUI
- **Isolation boundary** — Each agent's tool execution occurs in an isolated environment (Docker, Bubblewrap, or native sandbox)

## Persistence

- **Agent Registry** — SQLite table `agent_registry` stores all trusted agents with metadata (registered_by, created_at, capabilities snapshot)
- **Configuration** — `~/.config/navi/agents/<name>/config.toml` and `AGENT.md` (config as code)
- **Event Log** — All agent actions, tool calls, auth events are recorded for audit and replay

## Observability

- **Events** — `agent.loaded`, `agent.created`, `agent.removed`, `tool_call.<name>`, `auth.request`, `auth.success`, `auth.failure`
- **Metrics** — per-agent task count, duration, error rate, tool call success rate
- **Tracing** — Future: distributed trace across orchestrator → agent → tools

## Comparison: Registered vs Unregistered Agents

| Aspect | Registered Agent | Unregistered Agent |
|--------|------------------|--------------------|
| Exists in `agent_registry` | Yes | No |
| `is_trusted = true` | Yes | No |
| Can call privileged tools | After user auth | Never |
| Can call unprivileged tools | Yes (within capabilities) | No |
| Audit trail | Full (user_id, agent_id) | Minimal (alert only) |
| UI visibility | Appears in agent list | Hidden; only seen in alerts |

## Future Considerations

- **Delegated sub-agents** — An agent may spawn a child agent with reduced capabilities
- **Agent marketplace** — Signed agent packages that auto-register upon user approval
- **Revocation** — Ability to mark an agent as `revoked` without deleting it
- **Capability delegation** — Temporary grant of an extra capability for a single task (with auth)