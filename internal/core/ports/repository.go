package ports

import (
	"context"
	"errors"
	"navi/internal/core/domain"
)

var (
	ErrNotFound = errors.New("not found")
)

type EventLog interface {
	Record(ctx context.Context, event domain.Event) error
	Query(ctx context.Context, filter EventFilter) ([]domain.Event, error)
	Subscribe(ctx context.Context, eventTypes []domain.EventType) (<-chan domain.Event, error)
	Close() error
}

type EventFilter struct {
	AgentID   domain.AgentID
	UserID    string
	Type      domain.EventType
	StartTime int64
	EndTime   int64
	Limit     int
	Offset    int
}

type EventRepository interface {
	Save(ctx context.Context, event domain.Event) error
	FindByID(id string) (domain.Event, error)
	FindAll(ctx context.Context, filter EventFilter) ([]domain.Event, error)
}

type AgentRepository interface {
	Save(ctx context.Context, agent domain.Agent) error
	FindByID(id domain.AgentID) (domain.Agent, error)
	FindAll(ctx context.Context) ([]domain.Agent, error)
	Delete(ctx context.Context, id domain.AgentID) error
}
