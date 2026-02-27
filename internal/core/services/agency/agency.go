package agency

import (
	"context"
	"fmt"
	"time"
	"navi/internal/core/domain"
	"navi/internal/core/ports"
)

type Agency struct {
	agentRepo ports.AgentRepository
	eventLog  ports.EventLog
	agents    map[domain.AgentID]domain.Agent
}

func NewAgency(agentRepo ports.AgentRepository, eventLog ports.EventLog) *Agency {
	return &Agency{
		agentRepo: agentRepo,
		eventLog:  eventLog,
		agents:    make(map[domain.AgentID]domain.Agent),
	}
}

func (a *Agency) RegisterAgent(ctx context.Context, cfg domain.AgentConfig) error {
	// TODO: build agent from config, persist to agentRepo
	// For now: simple stub
	agent := domain.NewGenericAgent(cfg)
	if err := a.agentRepo.Save(ctx, agent); err != nil {
		return err
	}
	a.agents[agent.ID()] = agent

	event := domain.Event{
		ID:        fmt.Sprintf("evt-%s", agent.ID()),
		Timestamp: time.Now(),
		AgentID:   agent.ID(),
		UserID:    "system",
		Type:      domain.EventAgentCreated,
	}
	return a.eventLog.Record(ctx, event)
}

func (a *Agency) AssignTask(ctx context.Context, task domain.Task) error {
	agent, ok := a.agents[task.AgentID]
	if !ok {
		return fmt.Errorf("agent not found: %s", task.AgentID)
	}

	if !agent.CanHandle(task) {
		return fmt.Errorf("agent cannot handle task: %s", task.ID)
	}

	// Execute in background or sync?
	// For simplicity, sync:
	result, err := agent.Execute(ctx, task)
	if err != nil {
		event := domain.Event{
			ID:        fmt.Sprintf("evt-%s-fail", task.ID),
			Timestamp: time.Now(),
			AgentID:   task.AgentID,
			UserID:    "system",
			Type:      domain.EventTaskCompleted,
			Error:     err.Error(),
		}
		a.eventLog.Record(ctx, event)
		return err
	}

	event := domain.Event{
		ID:        fmt.Sprintf("evt-%s-ok", task.ID),
		Timestamp: time.Now(),
		AgentID:   task.AgentID,
		UserID:    "system",
		Type:      domain.EventTaskCompleted,
		Result:    result.Output,
	}
	a.eventLog.Record(ctx, event)
	return nil
}

func (a *Agency) ListAgents() []domain.Agent {
	list := make([]domain.Agent, 0, len(a.agents))
	for _, agent := range a.agents {
		list = append(list, agent)
	}
	return list
}
