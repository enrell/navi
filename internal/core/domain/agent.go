package domain

import (
	"context"
	"time"
)

type Agent interface {
	ID() AgentID
	Config() AgentConfig
	Role() AgentRole
	IsTrusted() bool
	CanHandle(task Task) bool
	Execute(ctx context.Context, task Task) (TaskResult, error)
	CallTool(ctx context.Context, call ToolCall) (ToolResponse, error)
}

type InMemoryAgentRegistry struct {
	agents map[AgentID]Agent
}

func NewInMemoryAgentRegistry() *InMemoryAgentRegistry {
	return &InMemoryAgentRegistry{
		agents: make(map[AgentID]Agent),
	}
}

func (r *InMemoryAgentRegistry) Add(agent Agent) error {
	r.agents[agent.ID()] = agent
	return nil
}

func (r *InMemoryAgentRegistry) Remove(id AgentID) error {
	delete(r.agents, id)
	return nil
}

func (r *InMemoryAgentRegistry) Get(id AgentID) (Agent, bool) {
	a, ok := r.agents[id]
	return a, ok
}

func (r *InMemoryAgentRegistry) List() []Agent {
	result := make([]Agent, 0, len(r.agents))
	for _, a := range r.agents {
		result = append(result, a)
	}
	return result
}

func (r *InMemoryAgentRegistry) Trusted() []Agent {
	result := make([]Agent, 0)
	for _, a := range r.agents {
		if a.IsTrusted() {
			result = append(result, a)
		}
	}
	return result
}

type GenericAgent struct {
	config AgentConfig
}

func NewGenericAgent(config AgentConfig) *GenericAgent {
	return &GenericAgent{config: config}
}

func (g *GenericAgent) ID() AgentID {
	return AgentID(g.config.Name)
}

func (g *GenericAgent) Config() AgentConfig {
	return g.config
}

func (g *GenericAgent) Role() AgentRole {
	return RoleCustom
}

func (g *GenericAgent) IsTrusted() bool {
	return true
}

func (g *GenericAgent) CanHandle(task Task) bool {
	return true
}

func (g *GenericAgent) Execute(ctx context.Context, task Task) (TaskResult, error) {
	return TaskResult{
		TaskID:      task.ID,
		AgentID:     g.ID(),
		Completed:   true,
		Output:      "stub output",
		StartedAt:   time.Now(),
		CompletedAt: time.Now(),
	}, nil
}

func (g *GenericAgent) CallTool(ctx context.Context, call ToolCall) (ToolResponse, error) {
	return ToolResponse{RequestID: call.RequestID}, nil
}
