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
	"navi/internal/telemetry"
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
	ctx, traceID := telemetry.EnsureTraceID(ctx)
	telemetry.Logger().Info("task_create_start", "trace_id", traceID, "agent_id", agentID, "prompt_chars", len(prompt))
	if prompt == "" {
		telemetry.Logger().Error("task_create_invalid_prompt", "trace_id", traceID)
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
		telemetry.Logger().Error("task_initial_save_failed", "trace_id", traceID, "task_id", task.ID, "error", err.Error())
		return nil, fmt.Errorf("task: initial save: %w", err)
	}

	// Execute synchronously — an async worker queue is a future improvement.
	output, chatErr := s.chat.Chat(ctx, prompt)
	if chatErr != nil {
		telemetry.Logger().Error("task_chat_failed", "trace_id", traceID, "task_id", task.ID, "error", chatErr.Error())
		task.Status = domain.TaskStatusFailed
		task.Error = chatErr.Error()
	} else {
		telemetry.Logger().Info("task_chat_completed", "trace_id", traceID, "task_id", task.ID, "output_chars", len(output))
		task.Status = domain.TaskStatusCompleted
		task.Output = output
	}

	if err := s.tasks.Save(ctx, task); err != nil {
		telemetry.Logger().Error("task_result_save_failed", "trace_id", traceID, "task_id", task.ID, "error", err.Error())
		// Return the task as-is; the caller still gets a useful object even if
		// the final persist failed.
		return task, fmt.Errorf("task: result save: %w", err)
	}
	telemetry.Logger().Info("task_create_done", "trace_id", traceID, "task_id", task.ID, "status", string(task.Status))

	return task, nil
}

// Get retrieves a single task by ID.
func (s *Service) Get(ctx context.Context, id string) (*domain.Task, error) {
	traceID := telemetry.TraceID(ctx)
	t, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		telemetry.Logger().Error("task_get_failed", "trace_id", traceID, "task_id", id, "error", err.Error())
		return nil, err // ErrNotFound propagates unchanged for HTTP layer detection
	}
	telemetry.Logger().Info("task_get_done", "trace_id", traceID, "task_id", id)
	return t, nil
}

// List returns all tasks ordered by creation time.
func (s *Service) List(ctx context.Context) ([]*domain.Task, error) {
	traceID := telemetry.TraceID(ctx)
	out, err := s.tasks.FindAll(ctx)
	if err != nil {
		telemetry.Logger().Error("task_list_failed", "trace_id", traceID, "error", err.Error())
		return nil, err
	}
	telemetry.Logger().Info("task_list_done", "trace_id", traceID, "count", len(out))
	return out, nil
}

// newID generates a time-sortable task ID: "20060102150405-xxxxxxxx".
func newID() string {
	ts := time.Now().UTC().Format("20060102150405")
	suffix := uuid.New().String()[:8]
	return ts + "-" + suffix
}
