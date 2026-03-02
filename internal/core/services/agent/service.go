// Package agent implements the use-case layer for agent management.
package agent

import (
	"context"
	"fmt"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
)

// Service handles agent query operations.
type Service struct {
	agents ports.AgentRepository
}

// New wires an AgentRepository into a Service.
func New(agents ports.AgentRepository) *Service {
	return &Service{agents: agents}
}

// Get retrieves a single agent by ID.
func (s *Service) Get(ctx context.Context, id string) (*domain.Agent, error) {
	a, err := s.agents.FindByID(ctx, id)
	if err != nil {
		return nil, err // ErrNotFound propagates unchanged
	}
	return a, nil
}

// List returns all registered agents.
func (s *Service) List(ctx context.Context) ([]*domain.Agent, error) {
	agents, err := s.agents.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: list: %w", err)
	}
	return agents, nil
}
