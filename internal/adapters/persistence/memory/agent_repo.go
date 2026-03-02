package memory

import (
	"context"
	"fmt"

	"navi/internal/core/domain"
)

// AgentRepository is an in-memory AgentRepository seeded with a fixed list.
// Agents are immutable after construction — this mirrors the file-backed agent
// model where runtime writes are not expected.
type AgentRepository struct {
	agents []*domain.Agent
}

// NewAgentRepository seeds the repository with the provided agents.
// Passing nil or an empty slice is valid (returns an empty list).
func NewAgentRepository(agents []*domain.Agent) *AgentRepository {
	if agents == nil {
		agents = []*domain.Agent{}
	}
	// Defensive copy of the slice so callers cannot mutate it.
	stored := make([]*domain.Agent, len(agents))
	for i, a := range agents {
		cp := *a
		stored[i] = &cp
	}
	return &AgentRepository{agents: stored}
}

// FindByID returns a defensive copy of the agent with the given ID.
// Returns a wrapped domain.ErrNotFound if the ID is absent.
func (r *AgentRepository) FindByID(_ context.Context, id string) (*domain.Agent, error) {
	for _, a := range r.agents {
		if a.ID == id {
			cp := *a
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
}

// FindAll returns defensive copies of all agents.
func (r *AgentRepository) FindAll(_ context.Context) ([]*domain.Agent, error) {
	result := make([]*domain.Agent, len(r.agents))
	for i, a := range r.agents {
		cp := *a
		result[i] = &cp
	}
	return result, nil
}
