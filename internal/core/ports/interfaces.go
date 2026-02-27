package ports

import (
	"context"
	"errors"
	"navi/internal/core/domain"
)

var (
	ErrNotFound = errors.New("not found")
)

// ─── LLM ─────────────────────────────────────────────────────────────────────

// LLMPort is the single interface all LLM adapters must implement.
// Adapters live in internal/adapters/llm/*.
type LLMPort interface {
	Generate(ctx context.Context, prompt string) (string, error)
	Stream(ctx context.Context, prompt string, chunk func(string)) error
	Health(ctx context.Context) error
}

// ─── Isolation ────────────────────────────────────────────────────────────────

// IsolationPort is the security boundary agents use to perform side-effecting
// operations (run commands, read/write files).
// Adapters live in internal/adapters/isolation/*.
type IsolationPort interface {
	Execute(ctx context.Context, cmd string, args []string, env map[string]string) (exitCode int, stdout, stderr string, err error)
	ReadFile(ctx context.Context, path string) (string, error)
	WriteFile(ctx context.Context, path, content string) error
	Cleanup(ctx context.Context) error
}

// ─── Agent Config Registry ────────────────────────────────────────────────────

// AgentConfigRegistry loads and persists agent configurations from the
// file system (~/.config/navi/agents/).
// The concrete adapter is internal/adapters/registry/localfs.
type AgentConfigRegistry interface {
	// LoadAll scans the agents directory and returns all valid configs.
	LoadAll() ([]domain.AgentConfig, error)
	// Save writes config.toml and AGENT.md for the given config.
	Save(cfg domain.AgentConfig) error
	// Delete removes the agent directory.
	Delete(id string) error
}

// ─── Vector Store ─────────────────────────────────────────────────────────────

type VectorStore interface {
	Add(ctx context.Context, vector []float64, metadata map[string]string) (string, error)
	Search(ctx context.Context, query []float64, limit int) ([]SearchResult, error)
	Delete(ctx context.Context, id string) error
}

type SearchResult struct {
	ID       string
	Score    float64
	Metadata map[string]string
}

// ─── Event Log ────────────────────────────────────────────────────────────────

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

// ─── Repositories ─────────────────────────────────────────────────────────────

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
