package sqlite

import (
	"context"
	"fmt"
	"navi/internal/core/domain"
	"navi/internal/core/ports"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SQLiteRepository struct {
	db *gorm.DB
}

// ─── GORM Models ──────────────────────────────────────────────────────────────

type AgentRecord struct {
	ID            string `gorm:"primaryKey"`
	Name          string `gorm:"not null"`
	SystemPrompt  string `gorm:"not null"`
	Role          string `gorm:"not null"`
	Trusted       bool   `gorm:"not null"`
	Capabilities  string // comma-joined raw capability strings
	IsolationType string
	LLMProvider   string
	LLMModel      string
	Description   string
}

func (AgentRecord) TableName() string { return "agents" }

type EventRecord struct {
	ID                 string           `gorm:"primaryKey"`
	Timestamp          time.Time        `gorm:"not null"`
	AgentID            domain.AgentID   `gorm:"not null"`
	UserID             string           `gorm:"not null"`
	Type               domain.EventType `gorm:"not null"`
	CapabilityType     *string          `gorm:"column:capability_type"`
	CapabilityResource *string          `gorm:"column:capability_resource"`
	CapabilityMode     *string          `gorm:"column:capability_mode"`
	WorkspacePath      *string
	GitCommit          *string
	Result             *string
	Error              *string
	Metadata           *string
}

func (EventRecord) TableName() string { return "events" }

// ─── Constructor ──────────────────────────────────────────────────────────────

func NewSQLiteRepository(dsn string) (*SQLiteRepository, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}
	if err := db.AutoMigrate(&AgentRecord{}, &EventRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}
	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// ─── AgentRepository ─────────────────────────────────────────────────────────

func (r *SQLiteRepository) Save(ctx context.Context, agent domain.Agent) error {
	cfg := agent.Config()
	rawCaps := make([]string, len(cfg.Capabilities))
	for i, c := range cfg.Capabilities {
		rawCaps[i] = c.Raw()
	}
	rec := AgentRecord{
		ID:            string(agent.ID()),
		Name:          cfg.Name,
		SystemPrompt:  cfg.SystemPrompt,
		Role:          string(agent.Role()),
		Trusted:       agent.IsTrusted(),
		Capabilities:  strings.Join(rawCaps, ","),
		IsolationType: cfg.IsolationType,
		LLMProvider:   cfg.LLMProvider,
		LLMModel:      cfg.LLMModel,
		Description:   cfg.Description,
	}
	return r.db.WithContext(ctx).Save(&rec).Error
}

func (r *SQLiteRepository) FindByID(id domain.AgentID) (domain.Agent, error) {
	var rec AgentRecord
	if err := r.db.First(&rec, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return domain.NewGenericAgentStub(recordToConfig(rec)), nil
}

func (r *SQLiteRepository) FindAll(ctx context.Context) ([]domain.Agent, error) {
	var records []AgentRecord
	if err := r.db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, err
	}
	agents := make([]domain.Agent, len(records))
	for i, rec := range records {
		agents[i] = domain.NewGenericAgentStub(recordToConfig(rec))
	}
	return agents, nil
}

func (r *SQLiteRepository) Delete(ctx context.Context, id domain.AgentID) error {
	return r.db.WithContext(ctx).Delete(&AgentRecord{}, "id = ?", id).Error
}

func recordToConfig(rec AgentRecord) domain.AgentConfig {
	cfg := domain.AgentConfig{
		ID:            rec.ID,
		Name:          rec.Name,
		Description:   rec.Description,
		SystemPrompt:  rec.SystemPrompt,
		IsolationType: rec.IsolationType,
		LLMProvider:   rec.LLMProvider,
		LLMModel:      rec.LLMModel,
	}
	if rec.Capabilities != "" {
		for _, raw := range strings.Split(rec.Capabilities, ",") {
			if raw == "" {
				continue
			}
			c, err := domain.ParseCapability(raw)
			if err == nil {
				cfg.Capabilities = append(cfg.Capabilities, c)
			}
		}
	}
	return cfg
}

// ─── EventLog ─────────────────────────────────────────────────────────────────

func (r *SQLiteRepository) Record(ctx context.Context, event domain.Event) error {
	return r.saveEvent(ctx, event)
}

func (r *SQLiteRepository) Query(ctx context.Context, filter ports.EventFilter) ([]domain.Event, error) {
	return r.findAllEvents(ctx, filter)
}

func (r *SQLiteRepository) Subscribe(ctx context.Context, eventTypes []domain.EventType) (<-chan domain.Event, error) {
	// TODO: implement real pub/sub (polling or SQLite triggers)
	ch := make(chan domain.Event)
	close(ch)
	return ch, nil
}

// ─── EventRepository ─────────────────────────────────────────────────────────

func (r *SQLiteRepository) saveEvent(ctx context.Context, event domain.Event) error {
	rec := EventRecord{
		ID:        event.ID,
		Timestamp: event.Timestamp,
		AgentID:   event.AgentID,
		UserID:    event.UserID,
		Type:      event.Type,
	}
	if event.Capability != nil {
		rec.CapabilityType = &event.Capability.Type
		rec.CapabilityResource = &event.Capability.Resource
		rec.CapabilityMode = &event.Capability.Mode
	}
	setIfNonEmpty := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	rec.WorkspacePath = setIfNonEmpty(event.WorkspacePath)
	rec.GitCommit = setIfNonEmpty(event.GitCommit)
	rec.Result = setIfNonEmpty(event.Result)
	rec.Error = setIfNonEmpty(event.Error)
	return r.db.WithContext(ctx).Create(&rec).Error
}

func (r *SQLiteRepository) findAllEvents(ctx context.Context, filter ports.EventFilter) ([]domain.Event, error) {
	query := r.db.WithContext(ctx).Model(&EventRecord{})
	if filter.AgentID != "" {
		query = query.Where("agent_id = ?", filter.AgentID)
	}
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	if filter.StartTime > 0 {
		query = query.Where("timestamp >= ?", time.Unix(filter.StartTime, 0))
	}
	if filter.EndTime > 0 {
		query = query.Where("timestamp <= ?", time.Unix(filter.EndTime, 0))
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	var records []EventRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	events := make([]domain.Event, len(records))
	for i, rec := range records {
		events[i] = toDomainEvent(rec)
	}
	return events, nil
}

func toDomainEvent(rec EventRecord) domain.Event {
	evt := domain.Event{
		ID:        rec.ID,
		Timestamp: rec.Timestamp,
		AgentID:   rec.AgentID,
		UserID:    rec.UserID,
		Type:      rec.Type,
	}
	if rec.CapabilityType != nil {
		capResource := ""
		if rec.CapabilityResource != nil {
			capResource = *rec.CapabilityResource
		}
		capMode := ""
		if rec.CapabilityMode != nil {
			capMode = *rec.CapabilityMode
		}
		evt.Capability = &domain.Capability{
			Type:     *rec.CapabilityType,
			Resource: capResource,
			Mode:     capMode,
		}
	}
	if rec.WorkspacePath != nil {
		evt.WorkspacePath = *rec.WorkspacePath
	}
	if rec.GitCommit != nil {
		evt.GitCommit = *rec.GitCommit
	}
	if rec.Result != nil {
		evt.Result = *rec.Result
	}
	if rec.Error != nil {
		evt.Error = *rec.Error
	}
	return evt
}
