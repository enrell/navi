package ports

import "context"

// Tool describes a callable tool exposed to the orchestrator.
type Tool struct {
	Name        string
	Description string
}

// ToolExecutor lets the orchestrator discover and execute tools.
type ToolExecutor interface {
	ListTools(ctx context.Context) ([]Tool, error)
	ExecuteTool(ctx context.Context, name, input string) (string, error)
}
