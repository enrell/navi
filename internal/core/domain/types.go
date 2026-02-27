package domain

import (
	"fmt"
	"strings"
	"time"
)

// ─── Identifiers ────────────────────────────────────────────────────────────

type AgentID string

// ─── Capabilities ────────────────────────────────────────────────────────────

// Capability represents a fine-grained permission string.
// Raw format examples:
//
//	"filesystem:workspace:rw"
//	"exec:bash,go,git"
//	"network:api.github.com:443"
//	"tool:mcp-name"
//	"vision"
//	"ocr:tesseract"
type Capability struct {
	Type     string // "filesystem", "exec", "network", "tool", "vision", "ocr", "audio"
	Resource string // path, host, binary list, tool name, engine, mode
	Mode     string // "ro", "rw", port, or empty
}

// Raw returns the capability as its normalized string form.
func (c Capability) Raw() string {
	parts := []string{c.Type}
	if c.Resource != "" {
		parts = append(parts, c.Resource)
	}
	if c.Mode != "" {
		parts = append(parts, c.Mode)
	}
	return strings.Join(parts, ":")
}

// ParseCapability converts a raw capability string into a Capability struct.
func ParseCapability(s string) (Capability, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) == 0 || parts[0] == "" {
		return Capability{}, fmt.Errorf("empty capability string")
	}
	cap := Capability{Type: parts[0]}
	if len(parts) > 1 {
		cap.Resource = parts[1]
	}
	if len(parts) > 2 {
		cap.Mode = parts[2]
	}
	return cap, nil
}

// ─── Agent Configuration ─────────────────────────────────────────────────────

// AgentConfig is the in-memory representation of a loaded agent.
// It is populated from config.toml + AGENT.md on disk.
type AgentConfig struct {
	// Identity
	ID          string // unique, must match directory name
	Name        string // human-readable label (may equal ID)
	Description string
	Type        string // always "generic" for now

	// Prompt
	PromptFile   string // filename of AGENT.md (relative to agent dir)
	SystemPrompt string // loaded content of PromptFile

	// LLM
	LLMProvider    string
	LLMModel       string
	LLMAPIKey      string
	LLMBaseURL     string
	LLMTemperature float64
	LLMMaxTokens   int

	// Capabilities & isolation
	Capabilities    []Capability
	IsolationType   string // "docker", "bubblewrap", "native"
	IsolationConfig map[string]string

	// Limits
	Timeout       time.Duration
	MaxConcurrent int
}

// ─── Agent Messages ───────────────────────────────────────────────────────────

// AgentMessage is the envelope for all orchestrator<->agent communication.
type AgentMessage struct {
	From    AgentID
	To      AgentID
	Type    string      // "request", "response", "event", "error"
	Payload interface{} // TaskPayload | ResultPayload
}

// TaskPayload is sent from the orchestrator to an agent.
type TaskPayload struct {
	TaskID      string
	Description string
	Context     TaskContext
}

// ResultPayload is sent from an agent back to the orchestrator.
type ResultPayload struct {
	TaskID  string         `json:"task_id"`
	Output  string         `json:"output"`
	Files   []FileChange   `json:"files,omitempty"`
	Error   string         `json:"error,omitempty"`
	Success bool           `json:"success"`
}

// FileChange represents a file that the agent wants to create/modify.
type FileChange struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// TaskContext carries per-task context passed to the agent.
type TaskContext struct {
	TaskID       string
	Goal         string
	WorkspacePath string
	Capabilities []Capability
	History      []AgentMessage
}

// ─── Tasks ────────────────────────────────────────────────────────────────────

type Task struct {
	ID           string
	AgentID      AgentID // optional: direct routing
	Prompt       string
	Requirements []Capability // capabilities required to handle this task
	Context      map[string]any
	Priority     int
	CreatedAt    time.Time
}

type TaskResult struct {
	TaskID      string
	AgentID     AgentID
	Output      string
	ToolCalls   []ToolCall
	Completed   bool
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
}

// ─── Tool Calls ───────────────────────────────────────────────────────────────

type ToolCall struct {
	RequestID string         `json:"request_id"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
	AgentID   AgentID        `json:"agent_id"`
}

type ToolResponse struct {
	RequestID string         `json:"request_id"`
	Result    map[string]any `json:"result"`
	Error     string         `json:"error"`
}

// ─── Events ───────────────────────────────────────────────────────────────────

type EventType string

const (
	EventAgentLoaded   EventType = "agent.loaded"
	EventAgentCreated  EventType = "agent.created"
	EventAgentRemoved  EventType = "agent.removed"
	EventTaskAssigned  EventType = "task.assigned"
	EventTaskCompleted EventType = "task.completed"
	EventToolCall      EventType = "tool.call"
	EventToolResponse  EventType = "tool.response"
	EventAuthRequest   EventType = "auth.request"
	EventAuthSuccess   EventType = "auth.success"
	EventAuthFailure   EventType = "auth.failure"
)

type Event struct {
	ID            string         `json:"id"`
	Timestamp     time.Time      `json:"timestamp"`
	AgentID       AgentID        `json:"agent_id"`
	UserID        string         `json:"user_id"`
	Type          EventType      `json:"type"`
	Capability    *Capability    `json:"capability,omitempty"`
	WorkspacePath string         `json:"workspace_path,omitempty"`
	GitCommit     string         `json:"git_commit,omitempty"`
	Result        string         `json:"result,omitempty"`
	Error         string         `json:"error,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// ─── Roles (legacy, kept for compatibility) ───────────────────────────────────

type AgentRole string

const (
	RolePlanner    AgentRole = "planner"
	RoleCoder      AgentRole = "coder"
	RoleResearcher AgentRole = "researcher"
	RoleExecutor   AgentRole = "executor"
	RoleVerifier   AgentRole = "verifier"
	RoleCustom     AgentRole = "custom"
)
