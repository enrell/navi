package orchestrator

import (
	"context"
	"navi/internal/agent"
	"navi/internal/eventlog"
	"navi/pkg/types"
)

type SimpleOrchestrator struct {
	registry agent.AgentRegistry
	logger   eventlog.EventLog
}

func NewSimpleOrchestrator(registry agent.AgentRegistry, logger eventlog.EventLog) *SimpleOrchestrator {
	return &SimpleOrchestrator{registry: registry, logger: logger}
}

func (o *SimpleOrchestrator) RegisterAgent(ctx context.Context, config types.AgentConfig) error {
	return nil
}

func (o *SimpleOrchestrator) UnregisterAgent(ctx context.Context, agentID types.AgentID) error {
	return nil
}

func (o *SimpleOrchestrator) AssignTask(ctx context.Context, task types.Task) error {
	return nil
}

func (o *SimpleOrchestrator) GetAgent(id types.AgentID) (agent.Agent, error) {
	return nil, nil
}

func (o *SimpleOrchestrator) ListAgents() []agent.Agent {
	return o.registry.List()
}
