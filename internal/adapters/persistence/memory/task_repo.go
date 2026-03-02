// Package memory provides in-memory implementations of repository ports.
// Useful in tests and as a starting point before adding SQLite persistence.
package memory

import (
	"context"
	"fmt"
	"sync"

	"navi/internal/core/domain"
)

// TaskRepository is a thread-safe in-memory TaskRepository.
type TaskRepository struct {
	mu    sync.RWMutex
	byID  map[string]*domain.Task
	order []string // preserves insertion order for FindAll
}

// NewTaskRepository returns an empty TaskRepository.
func NewTaskRepository() *TaskRepository {
	return &TaskRepository{
		byID: make(map[string]*domain.Task),
	}
}

// Save creates or replaces the task keyed by its ID (upsert).
// A defensive copy is stored so callers cannot mutate the repository's state.
func (r *TaskRepository) Save(_ context.Context, task *domain.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := *task
	if _, exists := r.byID[task.ID]; !exists {
		r.order = append(r.order, task.ID)
	}
	r.byID[task.ID] = &cp
	return nil
}

// FindByID returns a defensive copy of the task with the given ID.
// Returns a wrapped domain.ErrNotFound if the ID is absent.
func (r *TaskRepository) FindByID(_ context.Context, id string) (*domain.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	cp := *t
	return &cp, nil
}

// FindAll returns defensive copies of all tasks in insertion order.
func (r *TaskRepository) FindAll(_ context.Context) ([]*domain.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*domain.Task, 0, len(r.order))
	for _, id := range r.order {
		cp := *r.byID[id]
		result = append(result, &cp)
	}
	return result, nil
}
