// Package orchestrator is the central message bus of Navi.
// It loads agents from the config registry, routes tasks to capable agents,
// and emits events to the event log.
package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
)

// Orchestrator manages the lifecycle of all GenericAgents and routes tasks.
type Orchestrator struct {
	mu       sync.RWMutex
	registry *domain.InMemoryAgentRegistry
	cfgReg   ports.AgentConfigRegistry
	eventLog ports.EventLog
	llmFn    LLMFactory
	isoFn    IsolationFactory
}

// LLMFactory creates an LLMPort adapter for a given AgentConfig.
type LLMFactory func(cfg domain.AgentConfig) (domain.LLMPort, error)

// IsolationFactory creates an IsolationPort adapter for a given AgentConfig.
type IsolationFactory func(cfg domain.AgentConfig) (domain.IsolationPort, error)

// New creates a new Orchestrator.
func New(
	cfgReg ports.AgentConfigRegistry,
	eventLog ports.EventLog,
	llmFn LLMFactory,
	isoFn IsolationFactory,
) *Orchestrator {
	return &Orchestrator{
		registry: domain.NewInMemoryAgentRegistry(),
		cfgReg:   cfgReg,
		eventLog: eventLog,
		llmFn:    llmFn,
		isoFn:    isoFn,
	}
}

// Start loads all agents from the config registry and starts them.
func (o *Orchestrator) Start(ctx context.Context) error {
	configs, err := o.cfgReg.LoadAll()
	if err != nil {
		return fmt.Errorf("orchestrator: load configs: %w", err)
	}

	for _, cfg := range configs {
		if err := o.startAgent(ctx, cfg); err != nil {
			// Log but don't abort — other agents should still start.
			_ = o.eventLog.Record(ctx, domain.Event{
				ID:        fmt.Sprintf("evt-err-%s-%d", cfg.ID, time.Now().UnixNano()),
				Timestamp: time.Now(),
				AgentID:   domain.AgentID(cfg.ID),
				UserID:    "system",
				Type:      domain.EventAgentRemoved,
				Error:     err.Error(),
			})
		}
	}
	return nil
}

// RegisterAgent dynamically adds a new agent at runtime.
// It persists the config to disk and starts the agent immediately.
func (o *Orchestrator) RegisterAgent(ctx context.Context, cfg domain.AgentConfig) error {
	if err := o.cfgReg.Save(cfg); err != nil {
		return fmt.Errorf("orchestrator: save config: %w", err)
	}
	if err := o.startAgent(ctx, cfg); err != nil {
		return err
	}
	return o.eventLog.Record(ctx, domain.Event{
		ID:        fmt.Sprintf("evt-created-%s-%d", cfg.ID, time.Now().UnixNano()),
		Timestamp: time.Now(),
		AgentID:   domain.AgentID(cfg.ID),
		UserID:    "system",
		Type:      domain.EventAgentCreated,
	})
}

func (o *Orchestrator) startAgent(ctx context.Context, cfg domain.AgentConfig) error {
	llm, err := o.llmFn(cfg)
	if err != nil {
		return fmt.Errorf("create LLM for agent %s: %w", cfg.ID, err)
	}
	iso, err := o.isoFn(cfg)
	if err != nil {
		return fmt.Errorf("create isolation for agent %s: %w", cfg.ID, err)
	}

	agent := domain.NewGenericAgent(cfg, llm, iso)
	agent.Start(ctx)
	o.registry.Add(agent)

	return o.eventLog.Record(ctx, domain.Event{
		ID:        fmt.Sprintf("evt-loaded-%s-%d", cfg.ID, time.Now().UnixNano()),
		Timestamp: time.Now(),
		AgentID:   agent.ID(),
		UserID:    "system",
		Type:      domain.EventAgentLoaded,
	})
}

// Submit routes a task to an idle capable agent and executes it.
func (o *Orchestrator) Submit(ctx context.Context, task domain.Task) (domain.TaskResult, error) {
	agent, ok := o.registry.FindIdle(task)
	if !ok {
		return domain.TaskResult{}, fmt.Errorf("orchestrator: no capable agent available for task %s", task.ID)
	}

	_ = o.eventLog.Record(ctx, domain.Event{
		ID:        fmt.Sprintf("evt-assigned-%s-%d", task.ID, time.Now().UnixNano()),
		Timestamp: time.Now(),
		AgentID:   agent.ID(),
		UserID:    "system",
		Type:      domain.EventTaskAssigned,
	})

	result, err := agent.Execute(ctx, task)

	evtType := domain.EventTaskCompleted
	evt := domain.Event{
		ID:        fmt.Sprintf("evt-done-%s-%d", task.ID, time.Now().UnixNano()),
		Timestamp: time.Now(),
		AgentID:   agent.ID(),
		UserID:    "system",
		Type:      evtType,
		Result:    result.Output,
	}
	if err != nil {
		evt.Error = err.Error()
	}
	_ = o.eventLog.Record(ctx, evt)

	return result, err
}

// Shutdown stops all agents gracefully.
func (o *Orchestrator) Shutdown() {
	for _, agent := range o.registry.List() {
		agent.Stop()
	}
}

// ListAgents returns all registered agents.
func (o *Orchestrator) ListAgents() []*domain.GenericAgent {
	return o.registry.List()
}

// RemoveAgent stops and removes an agent by ID.
func (o *Orchestrator) RemoveAgent(ctx context.Context, id domain.AgentID) error {
	agent, ok := o.registry.Get(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}
	agent.Stop()
	o.registry.Remove(id)

	if err := o.cfgReg.Delete(string(id)); err != nil {
		return fmt.Errorf("delete config for agent %s: %w", id, err)
	}

	return o.eventLog.Record(ctx, domain.Event{
		ID:        fmt.Sprintf("evt-removed-%s-%d", id, time.Now().UnixNano()),
		Timestamp: time.Now(),
		AgentID:   id,
		UserID:    "system",
		Type:      domain.EventAgentRemoved,
	})
}
