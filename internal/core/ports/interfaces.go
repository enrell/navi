package ports

import (
	"context"
)

type LLMProvider interface {
	Complete(ctx context.Context, prompt string, options map[string]any) (string, error)
	Stream(ctx context.Context, prompt string, options map[string]any, chunkHandler func(string)) error
	Embed(ctx context.Context, text string) ([]float64, error)
	Health(ctx context.Context) error
}

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

type Sandbox interface {
	Execute(ctx context.Context, cmd string, args []string, env map[string]string) (int, string, string, error)
	CopyFile(ctx context.Context, src, dst string) error
	ReadFile(ctx context.Context, path string) (string, error)
	WriteFile(ctx context.Context, path string, content string) error
	Cleanup(ctx context.Context) error
}
