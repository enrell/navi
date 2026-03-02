package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
)

// agentRecord is the SQLite persistence model for domain.Agent.
type agentRecord struct {
	ID           string `gorm:"primaryKey;size:255"`
	Name         string `gorm:"size:255;not null"`
	Description  string `gorm:"type:text"`
	Capabilities string `gorm:"type:text;not null"` // JSON array []string
	Status       string `gorm:"size:32;not null;index"`
}

func (agentRecord) TableName() string { return "agents" }

// AgentRepository is the SQLite implementation of ports.AgentRepository.
type AgentRepository struct {
	db *gorm.DB
}

var _ ports.AgentRepository = (*AgentRepository)(nil)

// NewAgentRepository opens (or creates) a SQLite database at dbPath,
// auto-migrates the agents table, and returns a ready repository.
func NewAgentRepository(dbPath string) (*AgentRepository, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("sqlite agent repo: db path must not be empty")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("sqlite agent repo: mkdir: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("sqlite agent repo: open: %w", err)
	}

	if err := db.AutoMigrate(&agentRecord{}); err != nil {
		return nil, fmt.Errorf("sqlite agent repo: migrate: %w", err)
	}

	return &AgentRepository{db: db}, nil
}

// Close releases the underlying SQL connection pool.
func (r *AgentRepository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("sqlite agent repo: db handle: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("sqlite agent repo: close: %w", err)
	}
	return nil
}

// Seed inserts or updates the provided agents by ID.
// Useful for bootstrapping defaults in tests or startup flows.
func (r *AgentRepository) Seed(ctx context.Context, agents []*domain.Agent) error {
	for _, a := range agents {
		rec, err := toAgentRecord(a)
		if err != nil {
			return err
		}
		if err := r.db.WithContext(ctx).Save(&rec).Error; err != nil {
			return fmt.Errorf("sqlite agent repo: seed %q: %w", a.ID, err)
		}
	}
	return nil
}

// FindByID returns an agent by ID or domain.ErrNotFound.
func (r *AgentRepository) FindByID(ctx context.Context, id string) (*domain.Agent, error) {
	var rec agentRecord
	err := r.db.WithContext(ctx).First(&rec, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite agent repo: find by id %q: %w", id, err)
	}

	a, err := toAgentDomain(rec)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// FindAll returns all agents ordered by ID ascending.
func (r *AgentRepository) FindAll(ctx context.Context) ([]*domain.Agent, error) {
	var rows []agentRecord
	if err := r.db.WithContext(ctx).
		Order("id asc").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sqlite agent repo: find all: %w", err)
	}

	out := make([]*domain.Agent, 0, len(rows))
	for _, row := range rows {
		a, err := toAgentDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, nil
}

func toAgentRecord(a *domain.Agent) (agentRecord, error) {
	caps, err := json.Marshal(a.Capabilities)
	if err != nil {
		return agentRecord{}, fmt.Errorf("sqlite agent repo: marshal capabilities for %q: %w", a.ID, err)
	}
	return agentRecord{
		ID:           a.ID,
		Name:         a.Name,
		Description:  a.Description,
		Capabilities: string(caps),
		Status:       string(a.Status),
	}, nil
}

func toAgentDomain(rec agentRecord) (domain.Agent, error) {
	var caps []string
	if err := json.Unmarshal([]byte(rec.Capabilities), &caps); err != nil {
		return domain.Agent{}, fmt.Errorf("sqlite agent repo: unmarshal capabilities for %q: %w", rec.ID, err)
	}
	return domain.Agent{
		ID:           rec.ID,
		Name:         rec.Name,
		Description:  rec.Description,
		Capabilities: caps,
		Status:       domain.AgentStatus(rec.Status),
	}, nil
}
