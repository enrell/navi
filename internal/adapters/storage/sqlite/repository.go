package sqlite

import (
	"context"
	"fmt"
	"navi/internal/core/domain"
	"navi/internal/core/ports"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SQLiteRepository struct {
	db *gorm.DB
}

// GORM models

type AgentRecord struct {
	ID           string `gorm:"primaryKey"`
	Name         string `gorm:"not null"`
	SystemPrompt string `gorm:"not null"`
	Role         string `gorm:"not null"`
	Trusted      bool   `gorm:"not null"`
}

func (AgentRecord) TableName() string {
	return "agents"
}

type EventRecord struct {
	ID                    string            `gorm:"primaryKey"`
	Timestamp             time.Time         `gorm:"not null"`
	AgentID               domain.AgentID    `gorm:"not null"`
	UserID                string            `gorm:"not null"`
	Type                  domain.EventType  `gorm:"not null"`
	CapabilityType        *string           `gorm:"column:capability_type"`
	CapabilityResource   *string           `gorm:"column:capability_resource"`
	CapabilityMode       *string           `gorm:"column:capability_mode"`
	WorkspacePath         *string
	GitCommit             *string
	Result                *string
	Error                 *string
	Metadata              *string
}

func (EventRecord) TableName() string {
	return "events"
}

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

// AgentRepository implementation

func (r *SQLiteRepository) Save(ctx context.Context, agent domain.Agent) error {
	rec := AgentRecord{
		ID:           string(agent.ID()),
		Name:         agent.Config().Name,
		SystemPrompt: agent.Config().SystemPrompt,
		Role:         string(agent.Role()),
		Trusted:      agent.IsTrusted(),
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
	return domain.NewGenericAgent(domain.AgentConfig{Name: rec.Name, SystemPrompt: rec.SystemPrompt}), nil
}

func (r *SQLiteRepository) FindAll(ctx context.Context) ([]domain.Agent, error) {
	var records []AgentRecord
	if err := r.db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, err
	}
	agents := make([]domain.Agent, len(records))
	for i, rec := range records {
		agents[i] = domain.NewGenericAgent(domain.AgentConfig{Name: rec.Name, SystemPrompt: rec.SystemPrompt})
	}
	return agents, nil
}

func (r *SQLiteRepository) Delete(ctx context.Context, id domain.AgentID) error {
	return r.db.WithContext(ctx).Delete(&AgentRecord{}, "id = ?", id).Error
}

// EventRepository implementation

func (r *SQLiteRepository) SaveEvent(ctx context.Context, event domain.Event) error {
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

	if event.WorkspacePath != "" {
		rec.WorkspacePath = &event.WorkspacePath
	}
	if event.GitCommit != "" {
		rec.GitCommit = &event.GitCommit
	}
	if event.Result != "" {
		rec.Result = &event.Result
	}
	if event.Error != "" {
		rec.Error = &event.Error
	}

	return r.db.WithContext(ctx).Create(&rec).Error
}

func (r *SQLiteRepository) FindEventByID(id string) (domain.Event, error) {
	var rec EventRecord
	if err := r.db.First(&rec, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return domain.Event{}, ports.ErrNotFound
		}
		return domain.Event{}, err
	}
	return toDomainEvent(rec), nil
}

func (r *SQLiteRepository) FindAllEvents(ctx context.Context, filter ports.EventFilter) ([]domain.Event, error) {
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

// EventLog implementation

func (r *SQLiteRepository) Record(ctx context.Context, event domain.Event) error {
	return r.SaveEvent(ctx, event)
}

func (r *SQLiteRepository) Query(ctx context.Context, filter ports.EventFilter) ([]domain.Event, error) {
	return r.FindAllEvents(ctx, filter)
}

func (r *SQLiteRepository) Subscribe(ctx context.Context, eventTypes []domain.EventType) (<-chan domain.Event, error) {
	// TODO: implement real subscription with polling or triggers
	// For now, return a closed channel
	ch := make(chan domain.Event)
	close(ch)
	return ch, nil
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
		evt.Capability = &domain.Capability{
			Type:     *rec.CapabilityType,
			Resource: *rec.CapabilityResource,
			Mode:     *rec.CapabilityMode,
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
