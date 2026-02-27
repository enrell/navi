# Security Model

Navi's security model is **non-negotiable** and designed to protect against agent mistakes, not just external attackers. This document details the security architecture, threat model, and implemented safeguards.

## Core Security Principles

1. **Human-in-Control**: The user is always in control; agents are assistive, not autonomous
2. **Capability-Based Authority**: No implicit access; every operation must be explicitly granted
3. **Mandatory Authentication**: All interactions must be authenticated
4. **Isolation by Default**: Agents run in sandboxed environments
5. **Complete Auditability**: Every action is logged and traceable
6. **Fail-Safe Defaults**: When in doubt, deny access

## Threat Model

### In Scope (Mitigated by Navi)

| Threat | Mitigation |
|--------|------------|
| **Accidental filesystem destruction** | Workspace isolation; host filesystem not directly accessible |
| **Unintended network exposure** | Network whitelist per capability; no unrestricted outbound |
| **Workspace corruption** | Git integration; every change tracked; rollback possible |
| **Silent privilege escalation** | Capabilities explicitly granted; no automatic escalation |
| **Credential leakage** | Credentials stored separately; injected as env vars with rotation |
| **Prompt injection attacks** | Authentication required; no trusted input paths |

### Out of Scope (Accepted Risks)

| Threat | Reason |
|--------|--------|
| **Kernel zero-days** | Cannot be mitigated at application level |
| **Compromised host** | Assume host is trusted; physical security out of scope |
| **Malicious user intent** | System protects against mistakes, not intentional abuse |
| **Side-channel attacks** | Not designed for high-security environments |

> **Important**: Navi protects against **agent mistakes**, not **hostile adversaries**.

## Isolation Backends

Navi supports multiple isolation strategies. Each provides different security/performance tradeoffs.

### Docker Isolation

**Security Level**: High
**Platform**: Cross-platform

```yaml
isolation:
  backend: docker
  image: navi-sandbox:latest
  network: none  # or whitelist
  memory: 512m
  cpu: 0.5
  readonly: true
```

- Container runs as non-root user
- Host filesystem mounted as single workspace volume
- Network isolated (unless whitelisted)
- Resource limits enforced
- Pros: Strong isolation, works on any platform with Docker
- Cons: Requires Docker daemon; heavier startup

### Bubblewrap (bwrap)

**Security Level**: High
**Platform**: Linux only

```yaml
isolation:
  backend: bubblewrap
  network: none
  bind:
    - /host/workspace:/workspace:rw
    - /usr/bin/bash:/usr/bin/bash:ro
  unshare:
    - pid
    - net
    - ipc
    - uts
```

- No daemon required
- Process-level sandboxing
- Seccomp filters syscalls
- Pros: Lightweight, fast, no daemon
- Cons: Linux-only; requires kernel support

### Native Restricted

**Security Level**: Medium
**Platform**: All platforms

```yaml
isolation:
  backend: native
  chroot: /path/to/workspace
  seccomp: /path/to/profile.json
  capabilities: []
```

- Runs directly on host with restrictions
- Limited syscall filtering (Seccomp on Linux, Seatbelt on macOS)
- No container overhead
- Pros: Fast, no dependencies
- Cons: Weaker isolation; only for trusted environments

## Capability-Based Authority

### What Are Capabilities?

A **capability** is an explicit grant to perform a specific operation. Similar to file descriptor capabilities in capability-based systems.

### Capability Types

#### Filesystem Capabilities

```json
{
  "filesystem": [
    "workspace:rw",           // Read-write to workspace
    "/etc/hosts:ro",          // Read-only to hosts file
    "/home/user/.ssh:ro"      // Read-only SSH keys
  ]
}
```

Paths are validated:
- Absolute paths must be explicitly listed
- `workspace:` is a special token for the current workspace
- No glob patterns; explicit only

#### Network Capabilities

```json
{
  "network": [
    "api.github.com:443",     // Specific host:port
    "*.npmjs.org:443",        // Wildcard for registries
    "0.0.0.0/0:80,443"        // CIDR notation (use cautiously)
  ]
}
```

Network access is controlled by:
- Host whitelist
- Port restrictions
- Protocol (TCP/UDP)

#### Execution Capabilities

```json
{
  "exec": [
    "/usr/bin/bash",          // Absolute path required
    "/usr/local/bin/node",
    "/usr/bin/git"
  ]
}
```

Agents cannot:
- Execute arbitrary commands
- Use shell metacharacters (unless explicitly allowed)
- Access system binaries without permission

#### Port Capabilities

For services that need to bind ports:

```json
{
  "ports": [
    3000,  // Bind to 0.0.0.0:3000
    8080   // Bind to 0.0.0.0:8080
  ]
}
```

### Capability Inheritance

Tasks can inherit capabilities from:
- The user's role
- The agent's type (e.g., `executor` agent gets execution caps)
- The operation being performed (e.g., `git` gets filesystem:workspace:rw)

```go
func (o *Orchestrator) getEffectiveCapabilities(ctx context.Context, task Task) (Capabilities, error) {
    userCaps := o.auth.GetUserCapabilities(ctx)
    agentCaps := o.getAgentCapabilities(task.AgentID)
    taskCaps := task.RequestedCapabilities

    // Intersection of all three
    effective := intersect(userCaps, agentCaps, taskCaps)
    return effective, nil
}
```

## Authentication & Authorization

### Authentication Methods

| Mode | Method | Use Case |
|------|--------|----------|
| **TUI** | Local token stored in `~/.config/navi/token` | Single-user workstations |
| **REST API** | JWT signed with HMAC-SHA256 | API clients, external integrations |
| **Discord** | OAuth2 with Discord; user ID stored | Community bots |
| **Telegram** | User ID + optional PIN | Mobile remote access |

All tokens/sessions have configurable TTLs and can be revoked.

### Authorization Model

Role-Based Access Control (RBAC) with capability inheritance:

```yaml
roles:
  admin:
    capabilities: ["*"]  # All capabilities
  developer:
    capabilities:
      - filesystem: ["workspace:rw"]
      - network: ["api.github.com:443", "registry.npmjs.org:443"]
      - exec: ["/usr/bin/bash", "/usr/bin/node"]
      - ports: [3000, 8080]
  readonly:
    capabilities:
      - filesystem: ["workspace:ro"]
      - network: ["api.github.com:443"]
```

Users are assigned roles. Agents run with the user's role context.

### Session Management

- Sessions are created upon successful authentication
- Session ID stored in secure cookie or Authorization header
- Sessions expire after inactivity (configurable)
- All session activity logged

## Audit Logging

### Event Structure

Every significant action creates an immutable event:

```go
type Event struct {
    ID        string    `json:"id"`        // UUID
    Timestamp time.Time `json:"timestamp"` // UTC
    UserID    string    `json:"user_id"`   // Who did it
    AgentID   string    `json:"agent_id"`  // Which agent
    Action    string    `json:"action"`    // What happened
    Resource  string    `json:"resource"`  // Affected resource
    Details   string    `json:"details"`   // JSON-encoded details
    Result    string    `json:"result"`    // success/error
    Error     string    `json:"error"`     // Optional error msg
    GitCommit string    `json:"git_commit"` // Workspace state
}
```

### What Gets Audited

- Every task submitted
- Every capability grant/check
- Every container/sandbox start/stop
- Every filesystem mutation (tracked via git)
- Every network request (if enabled)
- Authentication events (login, logout, session expiry)
- Configuration changes

### Storage

Events stored in SQLite with Write-Ahead Logging (WAL):

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

WAL enables:
- Concurrent readers
- Crash recovery
- Streaming replication (future)

### Retention

Events are retained indefinitely unless user configures rotation. Because workspace changes are tracked via git, the event log can be pruned older than X days if storage becomes a concern.

## Secure Configuration

### Secrets Management

- **Never** hardcode secrets in source
- Use environment variables for API keys:
  ```bash
  export NAVI_OPENAI_API_KEY="sk-..."
  ```
- Or use a secrets manager (HashiCorp Vault, AWS Secrets Manager)
- Configuration file should have permissions `0600` (owner read/write only)

### Secure Defaults

```yaml
security:
  require_authentication: true
  session_timeout: 3600  # 1 hour
  max_concurrent_tasks: 10
  enable_network_isolation: true
  enforce_capability_checks: true
  audit_all_events: true
  allow_privilege_escalation: false
```

### Security Headers (for REST API)

```go
router.Use(middleware.SecurityHeaders(map[string]string{
    "Content-Security-Policy": "default-src 'self'",
    "X-Frame-Options":        "DENY",
    "X-Content-Type-Options": "nosniff",
    "Strict-Transport-Security": "max-age=31536000",
}))
```

## Secure Development Practices

### Code Review Checklist

- [ ] All user input validated
- [ ] All operations capability-checked
- [ ] All external calls authenticated
- [ ] No secrets in logs
- [ ] SQL queries parameterized
- [ ] Error messages don't leak information
- [ ] No `panic()` in production code

### Dependency Management

- Regular `go mod tidy` and `go mod verify`
- Scan dependencies with `govulncheck`
- Pin versions; avoid floating `^` ranges
- Review security advisories

### Penetration Testing

- Conduct regular security audits
- Fuzz test isolation backends
- Verify capability bypasses don't exist
- Test privilege escalation paths

## Incident Response

If a security incident occurs:

1. **Contain**: Disable affected user accounts, stop running tasks
2. **Investigate**: Query event log for timeline; identify scope
3. **Remediate**: Patch vulnerabilities; rotate compromised credentials
4. **Recover**: Restore from git backups if workspace corrupted
5. **Post-mortem**: Document root cause; update threat model

## Security Checklist for Users

Before deploying Navi:

- [ ] All tokens/API keys stored securely
- [ ] `.env` file not committed to git
- [ ] Docker daemon secured (if using Docker)
- [ ] Filesystem permissions set correctly
- [ ] Network firewall rules in place
- [ ] Audit logging enabled and monitored
- [ ] Regular backups of `~/.config/navi/` and workspaces
- [ ] Users trained on capability model

## Reporting Vulnerabilities

If you discover a security issue, **please do not open a public GitHub issue**. Email: security@enrell.dev

Include:
- Detailed description of vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We will respond within 48 hours and aim to patch within 30 days.

## References

- [OWASP Cheat Sheet Series](https://cheatsheetseries.owasp.org/)
- [Go Security Guidelines](https://pkg.go.dev/crypto)
- [Docker Security Best Practices](https://docs.docker.com/engine/security/)
- [Bubblewrap Security](https://github.com/containers/bubblewrap/blob/master/security.md)
