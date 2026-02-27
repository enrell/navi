package agent

import (
	"context"
	"navi/pkg/types"
)

type Agent interface {
	ID() types.AgentID
	Config() types.AgentConfig
	IsTrusted() bool
	CanHandle(task types.Task) bool
	Execute(ctx context.Context, task types.Task) (types.TaskResult, error)
	CallTool(ctx context.Context, call types.ToolCall) (types.ToolResponse, error)
}

type AgentRegistry interface {
	Add(agent Agent) error
	Remove(id types.AgentID) error
	Get(id types.AgentID) (Agent, bool)
	List() []Agent
	Trusted() []Agent
}

type AgentNotFoundError struct{}
type AgentUnauthorizedError struct{}
