package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Agent interface {
	ID() AgentID
	Config() AgentConfig
	Role() AgentRole
	IsTrusted() bool
	CanHandle(task Task) bool
	Execute(ctx context.Context, task Task) (TaskResult, error)
	CallTool(ctx context.Context, call ToolCall) (ToolResponse, error)
}

type LLMPort interface {
	Generate(ctx context.Context, prompt string) (string, error)
	Stream(ctx context.Context, prompt string, chunk func(string)) error
}

type IsolationPort interface {
	Execute(ctx context.Context, cmd string, args []string, env map[string]string) (exitCode int, stdout, stderr string, err error)
	ReadFile(ctx context.Context, path string) (string, error)
	WriteFile(ctx context.Context, path, content string) error
	Cleanup(ctx context.Context) error
}

type Authorizer interface {
	CheckExec(binary string) error
	CheckFilesystem(reqPath string, action string, workspacePath string) error
	CheckNetwork(host, port string) error
}

type GenericAgent struct {
	mu        sync.RWMutex
	config    AgentConfig
	llm       LLMPort
	isolation IsolationPort
	auth      Authorizer

	inbox    chan AgentMessage
	outbox   chan AgentMessage
	cancelFn context.CancelFunc

	activeTasks int
}

func NewGenericAgent(config AgentConfig, llm LLMPort, isolation IsolationPort, auth Authorizer) *GenericAgent {
	return &GenericAgent{
		config:    config,
		llm:       llm,
		isolation: isolation,
		auth:      auth,
		inbox:     make(chan AgentMessage, 16),
		outbox:    make(chan AgentMessage, 16),
	}
}

func NewGenericAgentStub(config AgentConfig) *GenericAgent {
	return NewGenericAgent(config, nil, nil, nil)
}

func (g *GenericAgent) ID() AgentID         { return AgentID(g.config.ID) }
func (g *GenericAgent) Config() AgentConfig { return g.config }
func (g *GenericAgent) Role() AgentRole     { return RoleCustom }
func (g *GenericAgent) IsTrusted() bool     { return true }

func (g *GenericAgent) CanHandle(task Task) bool {
	if len(task.Requirements) == 0 {
		return true
	}
	for _, req := range task.Requirements {
		if !g.hasCap(req) {
			return false
		}
	}
	return true
}

func (g *GenericAgent) hasCap(req Capability) bool {
	for _, c := range g.config.Capabilities {
		if c.Type == req.Type {
			if req.Resource == "" || req.Resource == "*" {
				return true
			}
			if c.Resource == req.Resource || c.Resource == "*" {
				return true
			}
		}
	}
	return false
}

func (g *GenericAgent) Execute(ctx context.Context, task Task) (TaskResult, error) {
	start := time.Now()

	if g.llm == nil {
		return TaskResult{}, fmt.Errorf("agent %s: no LLM adapter configured", g.ID())
	}

	fullPrompt := g.buildPrompt(task.Prompt)

	resp, err := g.llmWithRetry(ctx, fullPrompt)
	if err != nil {
		return TaskResult{
			TaskID:      task.ID,
			AgentID:     g.ID(),
			Error:       err.Error(),
			StartedAt:   start,
			CompletedAt: time.Now(),
		}, err
	}

	var result ResultPayload
	if jsonErr := json.Unmarshal([]byte(resp), &result); jsonErr != nil {
		result = ResultPayload{
			TaskID:  task.ID,
			Output:  resp,
			Success: true,
		}
	}

	if g.isolation != nil && len(result.Files) > 0 {
		var wsPath string
		if vp, ok := task.Context["workspace_path"]; ok {
			wsPath, _ = vp.(string)
		}

		for _, f := range result.Files {
			if g.auth != nil {
				if authErr := g.auth.CheckFilesystem(f.Path, "write", wsPath); authErr != nil {
					return TaskResult{
						TaskID:      task.ID,
						AgentID:     g.ID(),
						Error:       fmt.Sprintf("access denied to %s: %v", f.Path, authErr),
						StartedAt:   start,
						CompletedAt: time.Now(),
					}, authErr
				}
			}

			if wErr := g.isolation.WriteFile(ctx, f.Path, f.Content); wErr != nil {
				return TaskResult{
					TaskID:      task.ID,
					AgentID:     g.ID(),
					Error:       fmt.Sprintf("write file %s: %v", f.Path, wErr),
					StartedAt:   start,
					CompletedAt: time.Now(),
				}, wErr
			}
		}
	}

	return TaskResult{
		TaskID:      task.ID,
		AgentID:     g.ID(),
		Output:      result.Output,
		Completed:   result.Success,
		Error:       result.Error,
		StartedAt:   start,
		CompletedAt: time.Now(),
	}, nil
}

func (g *GenericAgent) CallTool(ctx context.Context, call ToolCall) (ToolResponse, error) {
	if g.isolation == nil {
		return ToolResponse{RequestID: call.RequestID, Error: "no isolation adapter"}, nil
	}
	if g.auth != nil {
		if err := g.auth.CheckExec(call.ToolName); err != nil {
			return ToolResponse{RequestID: call.RequestID, Error: err.Error()}, nil
		}
	}
	args := make([]string, 0)
	if v, ok := call.Arguments["args"]; ok {
		if slice, ok := v.([]string); ok {
			args = slice
		}
	}
	exitCode, stdout, stderr, err := g.isolation.Execute(ctx, call.ToolName, args, nil)
	if err != nil {
		return ToolResponse{RequestID: call.RequestID, Error: err.Error()}, nil
	}
	return ToolResponse{
		RequestID: call.RequestID,
		Result: map[string]any{
			"exit_code": exitCode,
			"stdout":    stdout,
			"stderr":    stderr,
		},
	}, nil
}

func (g *GenericAgent) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	g.cancelFn = cancel
	go g.runLoop(ctx)
}

func (g *GenericAgent) Stop() {
	if g.cancelFn != nil {
		g.cancelFn()
	}
}

func (g *GenericAgent) Outbox() <-chan AgentMessage {
	return g.outbox
}

func (g *GenericAgent) Send(msg AgentMessage) {
	g.inbox <- msg
}

func (g *GenericAgent) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-g.inbox:
			if msg.Type != "request" {
				continue
			}
			payload, ok := msg.Payload.(TaskPayload)
			if !ok {
				continue
			}
			task := Task{
				ID:        payload.TaskID,
				AgentID:   g.ID(),
				Prompt:    payload.Description,
				CreatedAt: time.Now(),
			}
			result, err := g.Execute(ctx, task)
			resp := AgentMessage{
				From: g.ID(),
				To:   msg.From,
				Type: "response",
				Payload: ResultPayload{
					TaskID:  task.ID,
					Output:  result.Output,
					Success: result.Completed,
					Error:   result.Error,
				},
			}
			if err != nil {
				resp.Type = "error"
			}
			g.outbox <- resp
		}
	}
}

func (g *GenericAgent) buildPrompt(taskPrompt string) string {
	return fmt.Sprintf("%s\n\n---\n\nTask:\n%s", g.config.SystemPrompt, taskPrompt)
}

func (g *GenericAgent) llmWithRetry(ctx context.Context, prompt string) (string, error) {
	var (
		resp string
		err  error
	)
	for i := range 3 {
		resp, err = g.llm.Generate(ctx, prompt)
		if err == nil {
			return resp, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(i+1) * time.Second):
		}
	}
	return "", fmt.Errorf("max retries exceeded: %w", err)
}

type InMemoryAgentRegistry struct {
	mu     sync.RWMutex
	agents map[AgentID]*GenericAgent
}

func NewInMemoryAgentRegistry() *InMemoryAgentRegistry {
	return &InMemoryAgentRegistry{agents: make(map[AgentID]*GenericAgent)}
}

func (r *InMemoryAgentRegistry) Add(agent *GenericAgent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[agent.ID()] = agent
}

func (r *InMemoryAgentRegistry) Remove(id AgentID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, id)
}

func (r *InMemoryAgentRegistry) Get(id AgentID) (*GenericAgent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[id]
	return a, ok
}

func (r *InMemoryAgentRegistry) List() []*GenericAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*GenericAgent, 0, len(r.agents))
	for _, a := range r.agents {
		list = append(list, a)
	}
	return list
}

func (r *InMemoryAgentRegistry) FindIdle(task Task) (*GenericAgent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, a := range r.agents {
		if a.CanHandle(task) {
			return a, true
		}
	}
	return nil, false
}
