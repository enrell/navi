package orchestrator

import (
	"context"
	"navi/internal/agent"
	"navi/pkg/types"
	"testing"
	"time"
)

type FakeAgent struct {
	id         types.AgentID
	isTrusted  bool
	canHandle  bool
	executeErr error
}

func (f *FakeAgent) ID() types.AgentID                     { return f.id }
func (f *FakeAgent) Config() types.AgentConfig             { return types.AgentConfig{Name: string(f.id)} }
func (f *FakeAgent) IsTrusted() bool                       { return f.isTrusted }
func (f *FakeAgent) CanHandle(task types.Task) bool        { return f.canHandle }
func (f *FakeAgent) Execute(ctx context.Context, task types.Task) (types.TaskResult, error) {
	return types.TaskResult{TaskID: task.ID, AgentID: f.id, Completed: true}, f.executeErr
}
func (f *FakeAgent) CallTool(ctx context.Context, call types.ToolCall) (types.ToolResponse, error) {
	return types.ToolResponse{RequestID: call.RequestID}, nil
}

func TestOrchestrator_RegisterAndAssign(t *testing.T) {
	ctx := context.Background()

	fake := &FakeAgent{
		id:        "test-agent",
		isTrusted: true,
		canHandle: true,
	}

	reg := agent.NewInMemoryAgentRegistry()
	reg.Add(fake)
	orch := NewSimpleOrchestrator(reg, nil)

	task := types.Task{
		ID:        "task-1",
		AgentID:   "test-agent",
		Prompt:    "test task",
		CreatedAt: time.Now(),
	}

	err := orch.AssignTask(ctx, task)
	if err != nil {
		t.Fatalf("AssignTask failed: %v", err)
	}
}
