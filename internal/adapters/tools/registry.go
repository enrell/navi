package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"navi/internal/core/ports"
	"navi/internal/telemetry"
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
		telemetry.Logger().Error("tool_register_invalid_name")
		return fmt.Errorf("tools: name cannot be empty")
	}
	if handler == nil {
		telemetry.Logger().Error("tool_register_nil_handler", "tool", name)
		return fmt.Errorf("tools: handler cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = entry{description: strings.TrimSpace(description), handler: handler}
	telemetry.Logger().Info("tool_registered", "tool", name)
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
		telemetry.Logger().Error("tool_unknown", "tool", name)
		return "", fmt.Errorf("tools: unknown tool %q", name)
	}
	traceID := telemetry.TraceID(ctx)
	telemetry.Logger().Info("tool_execute", "trace_id", traceID, "tool", name, "input_chars", len(input))
	result, err := e.handler(ctx, input)
	if err != nil {
		telemetry.Logger().Error("tool_execute_failed", "trace_id", traceID, "tool", name, "error", err.Error())
		return "", err
	}
	telemetry.Logger().Info("tool_execute_done", "trace_id", traceID, "tool", name, "result_chars", len(result))
	return result, nil
}
