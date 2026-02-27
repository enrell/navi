package eventlog

import (
	"context"
	"navi/pkg/types"
)

type EventLog interface {
	Record(ctx context.Context, event types.Event) error
	Query(ctx context.Context, filter EventFilter) ([]types.Event, error)
	Subscribe(ctx context.Context, eventTypes []types.EventType) (<-chan types.Event, error)
	Close() error
}

type EventFilter struct {
	AgentID   types.AgentID
	UserID    string
	Type      types.EventType
	StartTime int64
	EndTime   int64
	Limit     int
	Offset    int
}

type EventStore interface {
	Append(event types.Event) error
	GetByID(id string) (types.Event, error)
	GetAll(filter EventFilter) ([]types.Event, error)
}
