package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"navi/internal/core/ports"
)

type Handler func(ctx context.Context, input string) (string, error)

type entry struct {
	description string
	handler     Handler
}

// Registry is a simple in-process tool registry.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]entry
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]entry)}
}

func (r *Registry) Register(name, description string, handler Handler) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("tools: name cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("tools: handler cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = entry{description: strings.TrimSpace(description), handler: handler}
	return nil
}

func (r *Registry) ListTools(_ context.Context) ([]ports.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ports.Tool, 0, len(names))
	for _, name := range names {
		out = append(out, ports.Tool{Name: name, Description: r.tools[name].description})
	}
	return out, nil
}

func (r *Registry) ExecuteTool(ctx context.Context, name, input string) (string, error) {
	r.mu.RLock()
	e, ok := r.tools[strings.TrimSpace(name)]
	r.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("tools: unknown tool %q", name)
	}
	return e.handler(ctx, input)
}
