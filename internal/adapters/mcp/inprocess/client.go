package inprocess

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
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
		return fmt.Errorf("mcp: name cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("mcp: handler cannot be nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.tools[name] = handler
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
		return "", fmt.Errorf("mcp: unknown tool %q", name)
	}
	return h(ctx, input)
}
