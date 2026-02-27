package eventlog

import (
	"context"
	"database/sql"
	"navi/pkg/types"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteEventLog struct {
	db *sql.DB
}

func NewSQLiteEventLog(path string) (*SQLiteEventLog, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	if err := initializeSchema(db); err != nil {
		return nil, err
	}

	return &SQLiteEventLog{db: db}, nil
}

func initializeSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			timestamp DATETIME NOT NULL,
			agent_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			type TEXT NOT NULL,
			capability_type TEXT,
			capability_resource TEXT,
			capability_mode TEXT,
			workspace_path TEXT,
			git_commit TEXT,
			result TEXT,
			error TEXT,
			metadata TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
		CREATE INDEX IF NOT EXISTS idx_events_agent_id ON events(agent_id);
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
	`)
	return err
}

func (l *SQLiteEventLog) Record(ctx context.Context, event types.Event) error {
	_, err := l.db.Exec(`
		INSERT INTO events (
			id, timestamp, agent_id, user_id, type,
			capability_type, capability_resource, capability_mode,
			workspace_path, git_commit, result, error, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		event.ID,
		event.Timestamp.Format(time.RFC3339),
		event.AgentID,
		event.UserID,
		string(event.Type),
		nilOrString(event.Capability, func(c types.Capability) string { return c.Type }),
		nilOrString(event.Capability, func(c types.Capability) string { return c.Resource }),
		nilOrString(event.Capability, func(c types.Capability) string { return c.Mode }),
		event.WorkspacePath,
		event.GitCommit,
		event.Result,
		event.Error,
		serializeMetadata(event.Metadata),
	)
	return err
}

func nilOrString[T any](v *T, f func(T) string) string {
	if v == nil {
		return ""
	}
	return f(*v)
}

func serializeMetadata(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	// TODO: proper JSON serialization
	return ""
}

func (l *SQLiteEventLog) Query(ctx context.Context, filter EventFilter) ([]types.Event, error) {
	return nil, nil
}

func (l *SQLiteEventLog) Subscribe(ctx context.Context, eventTypes []types.EventType) (<-chan types.Event, error) {
	return nil, nil
}

func (l *SQLiteEventLog) Close() error {
	return l.db.Close()
}
