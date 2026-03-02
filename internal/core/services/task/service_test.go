package task_test

import (
	"context"
	"errors"
	"testing"

	"navi/internal/adapters/persistence/memory"
	"navi/internal/core/domain"
	"navi/internal/core/ports"
	chatservice "navi/internal/core/services/chat"
	taskservice "navi/internal/core/services/task"
)

// stubLLM is a controllable LLMPort for unit tests.
type stubLLM struct {
	reply string
	err   error
}

func (s *stubLLM) Chat(_ context.Context, _ []domain.Message) (string, error) {
	return s.reply, s.err
}

var _ ports.LLMPort = (*stubLLM)(nil)

// newSvc builds a task.Service wired to an in-memory repo and stub LLM.
func newSvc(llm ports.LLMPort) (*taskservice.Service, *memory.TaskRepository) {
	repo := memory.NewTaskRepository()
	chat := chatservice.New(llm)
	return taskservice.New(repo, chat), repo
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCreate_HappyPath(t *testing.T) {
	svc, repo := newSvc(&stubLLM{reply: "PONG"})

	task, err := svc.Create(context.Background(), "PING", "")
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if task.Status != domain.TaskStatusCompleted {
		t.Errorf("status = %q, want completed", task.Status)
	}
	if task.Output != "PONG" {
		t.Errorf("output = %q, want PONG", task.Output)
	}
	if task.ID == "" {
		t.Error("ID must not be empty")
	}

	// Verify the task was persisted.
	stored, err := repo.FindByID(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if stored.Status != domain.TaskStatusCompleted {
		t.Errorf("persisted status = %q, want completed", stored.Status)
	}
}

func TestCreate_EmptyPrompt_ReturnsError(t *testing.T) {
	svc, _ := newSvc(&stubLLM{reply: "x"})
	_, err := svc.Create(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestCreate_LLMError_TaskFailedStatus(t *testing.T) {
	llmErr := errors.New("rate limit exceeded")
	svc, _ := newSvc(&stubLLM{err: llmErr})

	task, err := svc.Create(context.Background(), "hello", "")
	if err != nil {
		t.Fatalf("Create must not return an error on LLM failure: %v", err)
	}
	if task.Status != domain.TaskStatusFailed {
		t.Errorf("status = %q, want failed", task.Status)
	}
	if task.Error == "" {
		t.Error("Error field must be set when LLM fails")
	}
}

func TestCreate_AgentID_IsStored(t *testing.T) {
	svc, _ := newSvc(&stubLLM{reply: "ok"})
	task, _ := svc.Create(context.Background(), "do something", "coder")
	if task.AgentID != "coder" {
		t.Errorf("AgentID = %q, want coder", task.AgentID)
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGet_ReturnsStoredTask(t *testing.T) {
	svc, _ := newSvc(&stubLLM{reply: "out"})
	created, _ := svc.Create(context.Background(), "query", "")

	got, err := svc.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
}

func TestGet_NotFound_ReturnsErrNotFound(t *testing.T) {
	svc, _ := newSvc(&stubLLM{reply: "x"})
	_, err := svc.Get(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound in chain", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestList_ReturnsAllTasks(t *testing.T) {
	svc, _ := newSvc(&stubLLM{reply: "ok"})
	_, _ = svc.Create(context.Background(), "first", "")
	_, _ = svc.Create(context.Background(), "second", "")

	tasks, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("len = %d, want 2", len(tasks))
	}
}

func TestList_EmptyRepo_ReturnsEmpty(t *testing.T) {
	svc, _ := newSvc(&stubLLM{reply: "x"})
	tasks, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("len = %d, want 0", len(tasks))
	}
}
