package domain

import (
	"time"
)

type AgentID string

type Capability struct {
	Type     string
	Resource string
	Mode     string
}

type AgentConfig struct {
	Name          string
	SystemPrompt  string
	Capabilities  []Capability
	IsolationType string
	LLMProvider   string
	LLMModel      string
	LLMAPIKey     string
	LLMBaseURL    string
}

type Task struct {
	ID        string
	AgentID   AgentID
	Prompt    string
	Context   map[string]any
	Priority  int
	CreatedAt time.Time
}

type TaskResult struct {
	TaskID       string
	AgentID      AgentID
	Output       string
	ToolCalls    []ToolCall
	Completed    bool
	Error        string
	StartedAt    time.Time
	CompletedAt  time.Time
}

type ToolCall struct {
	RequestID string                 `json:"request_id"`
	ToolName  string                 `json:"tool_name"`
	Arguments map[string]any         `json:"arguments"`
	AgentID   AgentID                `json:"agent_id"`
}

type ToolResponse struct {
	RequestID string                 `json:"request_id"`
	Result    map[string]any         `json:"result"`
	Error     string                 `json:"error"`
}

type EventType string

const (
	EventAgentLoaded    EventType = "agent.loaded"
	EventAgentCreated   EventType = "agent.created"
	EventAgentRemoved   EventType = "agent.removed"
	EventTaskAssigned   EventType = "task.assigned"
	EventTaskCompleted  EventType = "task.completed"
	EventToolCall       EventType = "tool.call"
	EventToolResponse   EventType = "tool.response"
	EventAuthRequest    EventType = "auth.request"
	EventAuthSuccess    EventType = "auth.success"
	EventAuthFailure    EventType = "auth.failure"
)

type Event struct {
	ID             string                 `json:"id"`
	Timestamp      time.Time              `json:"timestamp"`
	AgentID        AgentID                `json:"agent_id"`
	UserID         string                 `json:"user_id"`
	Type           EventType              `json:"type"`
	Capability     *Capability            `json:"capability,omitempty"`
	WorkspacePath  string                 `json:"workspace_path,omitempty"`
	GitCommit      string                 `json:"git_commit,omitempty"`
	Result         string                 `json:"result,omitempty"`
	Error          string                 `json:"error,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
}

type AgentRole string

const (
	RolePlanner    AgentRole = "planner"
	RoleCoder      AgentRole = "coder"
	RoleResearcher AgentRole = "researcher"
	RoleExecutor   AgentRole = "executor"
	RoleVerifier   AgentRole = "verifier"
	RoleCustom     AgentRole = "custom"
)
