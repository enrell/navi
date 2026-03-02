// Package task implements the use-case layer for task management.
// It orchestrates task creation, execution (via the chat port), and retrieval.
package task

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
	"navi/internal/core/services/chat"
)

// Service handles task lifecycle: create → execute → persist.
type Service struct {
	tasks ports.TaskRepository
	chat  *chat.Service
}

// New wires a TaskRepository and the chat use-case into a Service.
func New(tasks ports.TaskRepository, chat *chat.Service) *Service {
	return &Service{tasks: tasks, chat: chat}
}

// Create creates a task record, executes it synchronously via the chat
// service, persists the result, and returns the final task state.
func (s *Service) Create(ctx context.Context, prompt, agentID string) (*domain.Task, error) {
	if prompt == "" {
		return nil, fmt.Errorf("task: prompt must not be empty")
	}

	task := &domain.Task{
		ID:        newID(),
		AgentID:   agentID,
		Prompt:    prompt,
		Status:    domain.TaskStatusPending,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.tasks.Save(ctx, task); err != nil {
		return nil, fmt.Errorf("task: initial save: %w", err)
	}

	// Execute synchronously — an async worker queue is a future improvement.
	output, chatErr := s.chat.Chat(ctx, prompt)
	if chatErr != nil {
		task.Status = domain.TaskStatusFailed
		task.Error = chatErr.Error()
	} else {
		task.Status = domain.TaskStatusCompleted
		task.Output = output
	}

	if err := s.tasks.Save(ctx, task); err != nil {
		// Return the task as-is; the caller still gets a useful object even if
		// the final persist failed.
		return task, fmt.Errorf("task: result save: %w", err)
	}

	return task, nil
}

// Get retrieves a single task by ID.
func (s *Service) Get(ctx context.Context, id string) (*domain.Task, error) {
	t, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		return nil, err // ErrNotFound propagates unchanged for HTTP layer detection
	}
	return t, nil
}

// List returns all tasks ordered by creation time.
func (s *Service) List(ctx context.Context) ([]*domain.Task, error) {
	return s.tasks.FindAll(ctx)
}

// newID generates a time-sortable task ID: "20060102150405-xxxxxxxx".
func newID() string {
	ts := time.Now().UTC().Format("20060102150405")
	suffix := uuid.New().String()[:8]
	return ts + "-" + suffix
}
