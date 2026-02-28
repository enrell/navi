package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
)

type LLMFactory func(cfg domain.AgentConfig) (domain.LLMPort, error)
type IsolationFactory func(cfg domain.AgentConfig) (domain.IsolationPort, error)

type Orchestrator struct {
	mu       sync.RWMutex
	registry *domain.InMemoryAgentRegistry
	cfgReg   ports.AgentConfigRegistry
	eventLog ports.EventLog
	llmFn    LLMFactory
	isoFn    IsolationFactory
}

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

func (o *Orchestrator) Start(ctx context.Context) error {
	configs, err := o.cfgReg.LoadAll()
	if err != nil {
		return fmt.Errorf("orchestrator: load configs: %w", err)
	}

	for _, cfg := range configs {
		if err := o.startAgent(ctx, cfg); err != nil {
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

	evt := domain.Event{
		ID:        fmt.Sprintf("evt-done-%s-%d", task.ID, time.Now().UnixNano()),
		Timestamp: time.Now(),
		AgentID:   agent.ID(),
		UserID:    "system",
		Type:      domain.EventTaskCompleted,
		Result:    result.Output,
	}
	if err != nil {
		evt.Error = err.Error()
	}
	_ = o.eventLog.Record(ctx, evt)

	return result, err
}

func (o *Orchestrator) Shutdown() {
	for _, agent := range o.registry.List() {
		agent.Stop()
	}
}

func (o *Orchestrator) ListAgents() []*domain.GenericAgent {
	return o.registry.List()
}

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
