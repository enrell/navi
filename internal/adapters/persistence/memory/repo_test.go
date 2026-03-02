package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"navi/internal/adapters/persistence/memory"
	"navi/internal/core/domain"
)

var ctx = context.Background()

// ── TaskRepository ────────────────────────────────────────────────────────────

func TestTaskRepo_SaveAndFindByID(t *testing.T) {
	repo := memory.NewTaskRepository()
	task := &domain.Task{ID: "t1", Prompt: "hello", Status: domain.TaskStatusPending, CreatedAt: time.Now()}

	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	got, err := repo.FindByID(ctx, "t1")
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if got.Prompt != "hello" {
		t.Errorf("Prompt = %q, want hello", got.Prompt)
	}
}

func TestTaskRepo_Save_UpdatesExisting(t *testing.T) {
	repo := memory.NewTaskRepository()
	task := &domain.Task{ID: "t1", Status: domain.TaskStatusPending}
	_ = repo.Save(ctx, task)

	task.Status = domain.TaskStatusCompleted
	task.Output = "done"
	_ = repo.Save(ctx, task)

	got, _ := repo.FindByID(ctx, "t1")
	if got.Status != domain.TaskStatusCompleted {
		t.Errorf("status = %q, want completed", got.Status)
	}
	// Verify insertion order slice has no duplicates.
	all, _ := repo.FindAll(ctx)
	if len(all) != 1 {
		t.Errorf("FindAll len = %d, want 1", len(all))
	}
}

func TestTaskRepo_FindByID_NotFound(t *testing.T) {
	repo := memory.NewTaskRepository()
	_, err := repo.FindByID(ctx, "missing")
	if err == nil {
		t.Fatal("expected error for missing task")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound in chain", err)
	}
}

func TestTaskRepo_FindAll_Order(t *testing.T) {
	repo := memory.NewTaskRepository()
	for _, id := range []string{"a", "b", "c"} {
		_ = repo.Save(ctx, &domain.Task{ID: id})
	}

	all, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll error: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("len = %d, want 3", len(all))
	}
	if all[0].ID != "a" || all[1].ID != "b" || all[2].ID != "c" {
		t.Errorf("order = [%s %s %s], want [a b c]", all[0].ID, all[1].ID, all[2].ID)
	}
}

func TestTaskRepo_Save_DefensiveCopy(t *testing.T) {
	repo := memory.NewTaskRepository()
	task := &domain.Task{ID: "t1", Prompt: "original"}
	_ = repo.Save(ctx, task)

	// Mutate after save — should not affect the stored value.
	task.Prompt = "mutated"

	got, _ := repo.FindByID(ctx, "t1")
	if got.Prompt != "original" {
		t.Errorf("defensive copy broken: Prompt = %q, want original", got.Prompt)
	}
}

// ── AgentRepository ───────────────────────────────────────────────────────────

func TestAgentRepo_FindAll_Empty(t *testing.T) {
	repo := memory.NewAgentRepository(nil)
	agents, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("len = %d, want 0", len(agents))
	}
}

func TestAgentRepo_FindByID_Found(t *testing.T) {
	repo := memory.NewAgentRepository([]*domain.Agent{
		{ID: "coder", Name: "Coder"},
	})
	a, err := repo.FindByID(ctx, "coder")
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if a.Name != "Coder" {
		t.Errorf("Name = %q, want Coder", a.Name)
	}
}

func TestAgentRepo_FindByID_NotFound(t *testing.T) {
	repo := memory.NewAgentRepository(nil)
	_, err := repo.FindByID(ctx, "ghost")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound in chain", err)
	}
}

func TestAgentRepo_FindAll_DefensiveCopy(t *testing.T) {
	original := []*domain.Agent{{ID: "x", Name: "original"}}
	repo := memory.NewAgentRepository(original)

	// Mutate the original slice after construction.
	original[0].Name = "mutated"

	agents, _ := repo.FindAll(ctx)
	if agents[0].Name != "original" {
		t.Errorf("defensive copy broken: Name = %q, want original", agents[0].Name)
	}
}
