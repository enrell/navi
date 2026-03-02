package domain

import (
	"errors"
	"time"
)

// ErrNotFound is returned by repositories when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// Task represents a unit of work submitted to the orchestrator.
type Task struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	Prompt    string     `json:"prompt"`
	Status    TaskStatus `json:"status"`
	Output    string     `json:"output,omitempty"`
	Error     string     `json:"error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
