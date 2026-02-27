package agent

import (
	"navi/pkg/types"
)

type InMemoryAgentRegistry struct {
	agents map[types.AgentID]Agent
}

func NewInMemoryAgentRegistry() *InMemoryAgentRegistry {
	return &InMemoryAgentRegistry{
		agents: make(map[types.AgentID]Agent),
	}
}

func (r *InMemoryAgentRegistry) Add(agent Agent) error {
	r.agents[agent.ID()] = agent
	return nil
}

func (r *InMemoryAgentRegistry) Remove(id types.AgentID) error {
	delete(r.agents, id)
	return nil
}

func (r *InMemoryAgentRegistry) Get(id types.AgentID) (Agent, bool) {
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
