package inprocess

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"navi/internal/telemetry"
)

type Handler func(ctx context.Context, input string) (string, error)

// Client is a minimal in-process MCP-like tool client used for Sprint 1.
type Client struct {
	mu    sync.RWMutex
	tools map[string]Handler
}

func New() *Client {
	return &Client{tools: make(map[string]Handler)}
}

func (c *Client) Register(name string, handler Handler) error {
	name = strings.TrimSpace(name)
	if name == "" {
		telemetry.Logger().Error("mcp_register_invalid_name")
		return fmt.Errorf("mcp: name cannot be empty")
	}
	if handler == nil {
		telemetry.Logger().Error("mcp_register_nil_handler", "tool", name)
		return fmt.Errorf("mcp: handler cannot be nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.tools[name] = handler
	telemetry.Logger().Info("mcp_tool_registered", "tool", name)
	return nil
}

func (c *Client) ListTools(_ context.Context) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.tools))
	for n := range c.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

func (c *Client) CallTool(ctx context.Context, name, input string) (string, error) {
	c.mu.RLock()
	h, ok := c.tools[strings.TrimSpace(name)]
	c.mu.RUnlock()
	if !ok {
		telemetry.Logger().Error("mcp_tool_unknown", "tool", name)
		return "", fmt.Errorf("mcp: unknown tool %q", name)
	}
	traceID := telemetry.TraceID(ctx)
	telemetry.Logger().Info("mcp_tool_call", "trace_id", traceID, "tool", name, "input_chars", len(input))
	result, err := h(ctx, input)
	if err != nil {
		telemetry.Logger().Error("mcp_tool_call_failed", "trace_id", traceID, "tool", name, "error", err.Error())
		return "", err
	}
	telemetry.Logger().Info("mcp_tool_call_done", "trace_id", traceID, "tool", name, "result_chars", len(result))
	return result, nil
}
