package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	sqliteadapter "navi/internal/adapters/storage/sqlite"
	"navi/internal/core/domain"
)

func newRepo(t *testing.T) *sqliteadapter.TaskRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tasks.db")
	repo, err := sqliteadapter.NewTaskRepository(dbPath)
	if err != nil {
		t.Fatalf("NewTaskRepository error: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Fatalf("Close error: %v", err)
		}
	})
	return repo
}

func TestNewTaskRepository_EmptyPathError(t *testing.T) {
	_, err := sqliteadapter.NewTaskRepository("")
	if err == nil {
		t.Fatal("expected error for empty db path")
	}
}

func TestSaveAndFindByID(t *testing.T) {
	repo := newRepo(t)
	now := time.Now().UTC().Truncate(time.Second)
	task := &domain.Task{
		ID:        "20260302120000-abc12345",
		AgentID:   "coder",
		Prompt:    "hello",
		Status:    domain.TaskStatusPending,
		CreatedAt: now,
	}

	if err := repo.Save(context.Background(), task); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	got, err := repo.FindByID(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if got.Prompt != "hello" {
		t.Errorf("Prompt = %q, want hello", got.Prompt)
	}
	if got.Status != domain.TaskStatusPending {
		t.Errorf("Status = %q, want pending", got.Status)
	}
}

func TestSave_Upsert(t *testing.T) {
	repo := newRepo(t)
	task := &domain.Task{
		ID:        "20260302120000-upsert01",
		Prompt:    "v1",
		Status:    domain.TaskStatusPending,
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.Save(context.Background(), task); err != nil {
		t.Fatalf("first Save error: %v", err)
	}

	task.Status = domain.TaskStatusCompleted
	task.Output = "done"
	if err := repo.Save(context.Background(), task); err != nil {
		t.Fatalf("second Save error: %v", err)
	}

	got, err := repo.FindByID(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if got.Status != domain.TaskStatusCompleted {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.Output != "done" {
		t.Errorf("output = %q, want done", got.Output)
	}
}

func TestFindByID_NotFound(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.FindByID(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound in chain", err)
	}
}

func TestFindAll_OrderedByCreatedAtThenID(t *testing.T) {
	repo := newRepo(t)
	base := time.Now().UTC().Truncate(time.Second)

	tasks := []*domain.Task{
		{ID: "20260302120000-b", Prompt: "second", Status: domain.TaskStatusPending, CreatedAt: base},
		{ID: "20260302115959-a", Prompt: "first", Status: domain.TaskStatusPending, CreatedAt: base.Add(-time.Second)},
		{ID: "20260302120000-a", Prompt: "third", Status: domain.TaskStatusPending, CreatedAt: base},
	}
	for _, tsk := range tasks {
		if err := repo.Save(context.Background(), tsk); err != nil {
			t.Fatalf("Save %q error: %v", tsk.ID, err)
		}
	}

	all, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("FindAll error: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("len = %d, want 3", len(all))
	}

	gotOrder := []string{all[0].ID, all[1].ID, all[2].ID}
	wantOrder := []string{"20260302115959-a", "20260302120000-a", "20260302120000-b"}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("order[%d] = %q, want %q (full=%v)", i, gotOrder[i], wantOrder[i], gotOrder)
		}
	}
}

func TestFindAll_Empty(t *testing.T) {
	repo := newRepo(t)
	all, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("FindAll error: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("len = %d, want 0", len(all))
	}
}
