package ports

import (
	"context"

	"navi/internal/core/domain"
)

// TaskRepository is the persistence port for tasks.
// Adapters implement this — domain and services never import adapters.
type TaskRepository interface {
	// Save creates or updates a task (upsert by ID).
	Save(ctx context.Context, task *domain.Task) error

	// FindByID returns the task with the given ID.
	// Returns domain.ErrNotFound if no such task exists.
	FindByID(ctx context.Context, id string) (*domain.Task, error)

	// FindAll returns all tasks ordered by creation time (oldest first).
	FindAll(ctx context.Context) ([]*domain.Task, error)
}
