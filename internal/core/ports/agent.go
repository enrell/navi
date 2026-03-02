package ports

import (
	"context"

	"navi/internal/core/domain"
)

// AgentRepository is the persistence / registry port for agents.
type AgentRepository interface {
	// FindByID returns the agent with the given ID.
	// Returns domain.ErrNotFound if no such agent exists.
	FindByID(ctx context.Context, id string) (*domain.Agent, error)

	// FindAll returns all registered agents.
	FindAll(ctx context.Context) ([]*domain.Agent, error)
}
