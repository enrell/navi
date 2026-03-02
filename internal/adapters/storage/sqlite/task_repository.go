// Package sqlite provides SQLite-backed persistence adapters.
//
// This adapter implements the core TaskRepository port using GORM + SQLite.
// It lives in adapters/storage/sqlite to match the architecture docs.
package sqlite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
)

// taskRecord is the SQLite persistence model for domain.Task.
type taskRecord struct {
	ID        string `gorm:"primaryKey;size:64"`
	AgentID   string `gorm:"size:255"`
	Prompt    string `gorm:"type:text;not null"`
	Status    string `gorm:"size:32;not null;index"`
	Output    string `gorm:"type:text"`
	Error     string `gorm:"type:text"`
	CreatedAt time.Time
}

func (taskRecord) TableName() string { return "tasks" }

// TaskRepository is the SQLite implementation of ports.TaskRepository.
type TaskRepository struct {
	db *gorm.DB
}

var _ ports.TaskRepository = (*TaskRepository)(nil)

// NewTaskRepository opens (or creates) a SQLite database at dbPath,
// auto-migrates the tasks table, and returns a ready repository.
func NewTaskRepository(dbPath string) (*TaskRepository, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("sqlite task repo: db path must not be empty")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("sqlite task repo: mkdir: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("sqlite task repo: open: %w", err)
	}

	if err := db.AutoMigrate(&taskRecord{}); err != nil {
		return nil, fmt.Errorf("sqlite task repo: migrate: %w", err)
	}

	return &TaskRepository{db: db}, nil
}

// Close releases the underlying SQL connection pool.
// Call this in tests or on graceful shutdown to ensure file handles are closed.
func (r *TaskRepository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("sqlite task repo: db handle: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("sqlite task repo: close: %w", err)
	}
	return nil
}

// Save upserts a task by ID.
func (r *TaskRepository) Save(ctx context.Context, task *domain.Task) error {
	rec := toRecord(task)
	if err := r.db.WithContext(ctx).Save(&rec).Error; err != nil {
		return fmt.Errorf("sqlite task repo: save %q: %w", task.ID, err)
	}
	return nil
}

// FindByID returns a task by ID or domain.ErrNotFound.
func (r *TaskRepository) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	var rec taskRecord
	err := r.db.WithContext(ctx).First(&rec, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite task repo: find by id %q: %w", id, err)
	}
	t := toDomain(rec)
	return &t, nil
}

// FindAll returns all tasks ordered by CreatedAt ascending, then ID ascending.
func (r *TaskRepository) FindAll(ctx context.Context) ([]*domain.Task, error) {
	var rows []taskRecord
	if err := r.db.WithContext(ctx).
		Order("created_at asc").
		Order("id asc").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sqlite task repo: find all: %w", err)
	}

	out := make([]*domain.Task, 0, len(rows))
	for _, row := range rows {
		t := toDomain(row)
		out = append(out, &t)
	}
	return out, nil
}

func toRecord(t *domain.Task) taskRecord {
	return taskRecord{
		ID:        t.ID,
		AgentID:   t.AgentID,
		Prompt:    t.Prompt,
		Status:    string(t.Status),
		Output:    t.Output,
		Error:     t.Error,
		CreatedAt: t.CreatedAt,
	}
}

func toDomain(rec taskRecord) domain.Task {
	return domain.Task{
		ID:        rec.ID,
		AgentID:   rec.AgentID,
		Prompt:    rec.Prompt,
		Status:    domain.TaskStatus(rec.Status),
		Output:    rec.Output,
		Error:     rec.Error,
		CreatedAt: rec.CreatedAt,
	}
}
