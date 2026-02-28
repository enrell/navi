package domain

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type mockLLM struct {
	mu       sync.Mutex
	response string
	err      error
	calls    int
}

func (m *mockLLM) Generate(_ context.Context, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.response, m.err
}

func (m *mockLLM) Stream(_ context.Context, _ string, chunk func(string)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return m.err
	}
	chunk(m.response)
	return nil
}

type mockIsolation struct {
	mu          sync.Mutex
	writtenPath string
	writtenData string
	writeErr    error
	execExit    int
	execStdout  string
	execStderr  string
	execErr     error
}

func (m *mockIsolation) Execute(_ context.Context, _ string, _ []string, _ map[string]string) (int, string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.execExit, m.execStdout, m.execStderr, m.execErr
}

func (m *mockIsolation) ReadFile(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *mockIsolation) WriteFile(_ context.Context, path, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writtenPath = path
	m.writtenData = content
	return m.writeErr
}

func (m *mockIsolation) Cleanup(_ context.Context) error { return nil }

func testConfig() AgentConfig {
	return AgentConfig{
		ID:           "test-agent",
		Name:         "test-agent",
		Type:         "generic",
		SystemPrompt: "You are a test agent.",
		Capabilities: []Capability{
			{Type: "filesystem", Resource: "workspace", Mode: "rw"},
			{Type: "exec", Resource: "bash,go"},
		},
		IsolationType:  "native",
		LLMProvider:    "openai",
		LLMModel:       "gpt-4o-mini",
		LLMTemperature: 0.7,
		LLMMaxTokens:   4096,
		Timeout:        30 * time.Minute,
		MaxConcurrent:  5,
	}
}

func TestNewGenericAgent(t *testing.T) {
	llm := &mockLLM{}
	iso := &mockIsolation{}
	cfg := testConfig()
	agent := NewGenericAgent(cfg, llm, iso)

	if agent == nil {
		t.Fatal("NewGenericAgent returned nil")
	}
	if agent.llm != llm {
		t.Error("llm not set")
	}
	if agent.isolation != iso {
		t.Error("isolation not set")
	}
}

func TestNewGenericAgentStub(t *testing.T) {
	cfg := testConfig()
	agent := NewGenericAgentStub(cfg)

	if agent == nil {
		t.Fatal("NewGenericAgentStub returned nil")
	}
	if agent.llm != nil {
		t.Error("stub should have nil llm")
	}
	if agent.isolation != nil {
		t.Error("stub should have nil isolation")
	}
}

func TestGenericAgent_ID(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	if agent.ID() != "test-agent" {
		t.Errorf("ID() = %q, want test-agent", agent.ID())
	}
}

func TestGenericAgent_Config(t *testing.T) {
	cfg := testConfig()
	agent := NewGenericAgentStub(cfg)
	got := agent.Config()
	if got.ID != cfg.ID || got.Name != cfg.Name {
		t.Errorf("Config() mismatch")
	}
}

func TestGenericAgent_Role(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	if agent.Role() != RoleCustom {
		t.Errorf("Role() = %q, want custom", agent.Role())
	}
}

func TestGenericAgent_IsTrusted(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	if !agent.IsTrusted() {
		t.Error("IsTrusted() should return true")
	}
}

func TestGenericAgent_CanHandle_NoRequirements(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	task := Task{ID: "t1", Prompt: "do something"}
	if !agent.CanHandle(task) {
		t.Error("should handle task with no requirements")
	}
}

func TestGenericAgent_CanHandle_MatchingCaps(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	task := Task{
		ID: "t1",
		Requirements: []Capability{
			{Type: "filesystem", Resource: "workspace"},
		},
	}
	if !agent.CanHandle(task) {
		t.Error("should handle task with matching capabilities")
	}
}

func TestGenericAgent_CanHandle_MissingCaps(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	task := Task{
		ID: "t1",
		Requirements: []Capability{
			{Type: "vision"},
		},
	}
	if agent.CanHandle(task) {
		t.Error("should not handle task with missing capabilities")
	}
}

func TestGenericAgent_CanHandle_WildcardResource(t *testing.T) {
	cfg := testConfig()
	cfg.Capabilities = []Capability{
		{Type: "network", Resource: "*", Mode: "443"},
	}
	agent := NewGenericAgentStub(cfg)
	task := Task{
		ID: "t1",
		Requirements: []Capability{
			{Type: "network", Resource: "api.github.com"},
		},
	}
	if !agent.CanHandle(task) {
		t.Error("wildcard resource should match any specific resource")
	}
}

func TestGenericAgent_CanHandle_EmptyRequirementResource(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	task := Task{
		ID: "t1",
		Requirements: []Capability{
			{Type: "filesystem", Resource: ""},
		},
	}
	if !agent.CanHandle(task) {
		t.Error("empty requirement resource should match")
	}
}

func TestGenericAgent_CanHandle_WildcardRequirementResource(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	task := Task{
		ID: "t1",
		Requirements: []Capability{
			{Type: "filesystem", Resource: "*"},
		},
	}
	if !agent.CanHandle(task) {
		t.Error("wildcard requirement resource should match")
	}
}

func TestGenericAgent_Execute_NoLLM(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	_, err := agent.Execute(context.Background(), Task{ID: "t1", Prompt: "hello"})
	if err == nil {
		t.Fatal("expected error when LLM is nil")
	}
}

func TestGenericAgent_Execute_JSONResponse(t *testing.T) {
	llm := &mockLLM{response: `{"task_id":"t1","output":"done","success":true}`}
	agent := NewGenericAgent(testConfig(), llm, nil)

	result, err := agent.Execute(context.Background(), Task{ID: "t1", Prompt: "do it"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Completed {
		t.Error("expected Completed=true")
	}
	if result.Output != "done" {
		t.Errorf("Output = %q, want done", result.Output)
	}
	if result.AgentID != "test-agent" {
		t.Errorf("AgentID = %q", result.AgentID)
	}
	if result.TaskID != "t1" {
		t.Errorf("TaskID = %q", result.TaskID)
	}
}

func TestGenericAgent_Execute_RawResponse(t *testing.T) {
	llm := &mockLLM{response: "just some text"}
	agent := NewGenericAgent(testConfig(), llm, nil)

	result, err := agent.Execute(context.Background(), Task{ID: "t1", Prompt: "do it"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Completed {
		t.Error("expected Completed=true for raw fallback")
	}
	if result.Output != "just some text" {
		t.Errorf("Output = %q", result.Output)
	}
}

func TestGenericAgent_Execute_LLMError(t *testing.T) {
	llm := &mockLLM{err: errors.New("api down")}
	agent := NewGenericAgent(testConfig(), llm, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := agent.Execute(ctx, Task{ID: "t1", Prompt: "do it"})
	if err == nil {
		t.Fatal("expected error")
	}
	if result.Error == "" {
		t.Error("expected non-empty error in result")
	}
	if result.TaskID != "t1" {
		t.Errorf("TaskID = %q", result.TaskID)
	}
}

func TestGenericAgent_Execute_WithFileChanges(t *testing.T) {
	llm := &mockLLM{response: `{"task_id":"t1","output":"wrote file","success":true,"files":[{"path":"main.go","content":"package main"}]}`}
	iso := &mockIsolation{}
	agent := NewGenericAgent(testConfig(), llm, iso)

	result, err := agent.Execute(context.Background(), Task{ID: "t1", Prompt: "create main.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Completed {
		t.Error("expected Completed=true")
	}
	if iso.writtenPath != "main.go" {
		t.Errorf("writtenPath = %q, want main.go", iso.writtenPath)
	}
	if iso.writtenData != "package main" {
		t.Errorf("writtenData = %q", iso.writtenData)
	}
}

func TestGenericAgent_Execute_FileWriteError(t *testing.T) {
	llm := &mockLLM{response: `{"task_id":"t1","output":"ok","success":true,"files":[{"path":"bad.go","content":"x"}]}`}
	iso := &mockIsolation{writeErr: errors.New("disk full")}
	agent := NewGenericAgent(testConfig(), llm, iso)

	_, err := agent.Execute(context.Background(), Task{ID: "t1", Prompt: "write file"})
	if err == nil {
		t.Fatal("expected error from file write failure")
	}
}

func TestGenericAgent_Execute_JSONResponseWithError(t *testing.T) {
	llm := &mockLLM{response: `{"task_id":"t1","output":"","success":false,"error":"compilation failed"}`}
	agent := NewGenericAgent(testConfig(), llm, nil)

	result, err := agent.Execute(context.Background(), Task{ID: "t1", Prompt: "build"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Completed {
		t.Error("expected Completed=false")
	}
	if result.Error != "compilation failed" {
		t.Errorf("Error = %q", result.Error)
	}
}

func TestGenericAgent_Execute_TimestampsSet(t *testing.T) {
	llm := &mockLLM{response: "ok"}
	agent := NewGenericAgent(testConfig(), llm, nil)

	before := time.Now()
	result, err := agent.Execute(context.Background(), Task{ID: "t1", Prompt: "time test"})
	after := time.Now()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StartedAt.Before(before) || result.StartedAt.After(after) {
		t.Error("StartedAt out of range")
	}
	if result.CompletedAt.Before(result.StartedAt) {
		t.Error("CompletedAt before StartedAt")
	}
}

func TestGenericAgent_CallTool_NoIsolation(t *testing.T) {
	agent := NewGenericAgent(testConfig(), &mockLLM{}, nil)

	resp, err := agent.CallTool(context.Background(), ToolCall{RequestID: "r1", ToolName: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error != "no isolation adapter" {
		t.Errorf("Error = %q", resp.Error)
	}
}

func TestGenericAgent_CallTool_Success(t *testing.T) {
	iso := &mockIsolation{execExit: 0, execStdout: "hello", execStderr: ""}
	agent := NewGenericAgent(testConfig(), &mockLLM{}, iso)

	resp, err := agent.CallTool(context.Background(), ToolCall{
		RequestID: "r1",
		ToolName:  "echo",
		Arguments: map[string]any{"args": []string{"hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.RequestID != "r1" {
		t.Errorf("RequestID = %q", resp.RequestID)
	}
	if resp.Result["stdout"] != "hello" {
		t.Errorf("stdout = %v", resp.Result["stdout"])
	}
}

func TestGenericAgent_CallTool_ExecError(t *testing.T) {
	iso := &mockIsolation{execErr: errors.New("exec failed")}
	agent := NewGenericAgent(testConfig(), &mockLLM{}, iso)

	resp, err := agent.CallTool(context.Background(), ToolCall{RequestID: "r1", ToolName: "bad"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error != "exec failed" {
		t.Errorf("Error = %q", resp.Error)
	}
}

func TestGenericAgent_CallTool_NonStringArgs(t *testing.T) {
	iso := &mockIsolation{execStdout: "ok"}
	agent := NewGenericAgent(testConfig(), &mockLLM{}, iso)

	resp, err := agent.CallTool(context.Background(), ToolCall{
		RequestID: "r1",
		ToolName:  "echo",
		Arguments: map[string]any{"args": 42},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Result["exit_code"] != 0 {
		t.Errorf("exit_code = %v", resp.Result["exit_code"])
	}
}

func TestGenericAgent_CallTool_NoArgs(t *testing.T) {
	iso := &mockIsolation{execStdout: "ok"}
	agent := NewGenericAgent(testConfig(), &mockLLM{}, iso)

	resp, err := agent.CallTool(context.Background(), ToolCall{
		RequestID: "r1",
		ToolName:  "echo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %q", resp.Error)
	}
}

func TestGenericAgent_BuildPrompt(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	got := agent.buildPrompt("Do the thing")
	want := "You are a test agent.\n\n---\n\nTask:\nDo the thing"
	if got != want {
		t.Errorf("buildPrompt = %q, want %q", got, want)
	}
}

func TestGenericAgent_LLMWithRetry_SuccessFirstTry(t *testing.T) {
	llm := &mockLLM{response: "ok"}
	agent := NewGenericAgent(testConfig(), llm, nil)

	resp, err := agent.llmWithRetry(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %q", resp)
	}
	if llm.calls != 1 {
		t.Errorf("calls = %d, want 1", llm.calls)
	}
}

func TestGenericAgent_LLMWithRetry_MaxRetries(t *testing.T) {
	llm := &mockLLM{err: errors.New("transient")}
	agent := NewGenericAgent(testConfig(), llm, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := agent.llmWithRetry(ctx, "prompt")
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if llm.calls != 3 {
		t.Errorf("calls = %d, want 3", llm.calls)
	}
}

func TestGenericAgent_LLMWithRetry_ContextCancelled(t *testing.T) {
	llm := &mockLLM{err: errors.New("transient")}
	agent := NewGenericAgent(testConfig(), llm, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := agent.llmWithRetry(ctx, "prompt")
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestGenericAgent_Lifecycle(t *testing.T) {
	llm := &mockLLM{response: `{"task_id":"t1","output":"inbox-reply","success":true}`}
	agent := NewGenericAgent(testConfig(), llm, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agent.Start(ctx)

	agent.Send(AgentMessage{
		From:    "orchestrator",
		To:      agent.ID(),
		Type:    "request",
		Payload: TaskPayload{TaskID: "t1", Description: "hello from inbox"},
	})

	select {
	case msg := <-agent.Outbox():
		if msg.Type != "response" {
			t.Errorf("msg.Type = %q, want response", msg.Type)
		}
		payload, ok := msg.Payload.(ResultPayload)
		if !ok {
			t.Fatal("payload is not ResultPayload")
		}
		if payload.Output != "inbox-reply" {
			t.Errorf("payload.Output = %q", payload.Output)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for outbox message")
	}

	agent.Stop()
}

func TestGenericAgent_RunLoop_SkipsNonRequest(t *testing.T) {
	agent := NewGenericAgent(testConfig(), &mockLLM{response: "ok"}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent.Start(ctx)

	agent.Send(AgentMessage{
		From: "orchestrator",
		To:   agent.ID(),
		Type: "event",
	})

	select {
	case <-agent.Outbox():
		t.Fatal("should not receive outbox message for non-request")
	case <-time.After(200 * time.Millisecond):
	}

	agent.Stop()
}

func TestGenericAgent_RunLoop_SkipsBadPayload(t *testing.T) {
	agent := NewGenericAgent(testConfig(), &mockLLM{response: "ok"}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent.Start(ctx)

	agent.Send(AgentMessage{
		From:    "orchestrator",
		To:      agent.ID(),
		Type:    "request",
		Payload: "not a TaskPayload",
	})

	select {
	case <-agent.Outbox():
		t.Fatal("should not receive outbox message for bad payload")
	case <-time.After(200 * time.Millisecond):
	}

	agent.Stop()
}

func TestGenericAgent_RunLoop_LLMError(t *testing.T) {
	llm := &mockLLM{err: errors.New("api down")}
	agent := NewGenericAgent(testConfig(), llm, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	agent.Start(ctx)

	agent.Send(AgentMessage{
		From:    "orchestrator",
		To:      agent.ID(),
		Type:    "request",
		Payload: TaskPayload{TaskID: "t-err", Description: "fail"},
	})

	select {
	case msg := <-agent.Outbox():
		if msg.Type != "error" {
			t.Errorf("msg.Type = %q, want error", msg.Type)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for error response")
	}

	agent.Stop()
}

func TestGenericAgent_Stop_Nil(t *testing.T) {
	agent := NewGenericAgentStub(testConfig())
	agent.Stop()
}

func TestInMemoryAgentRegistry_AddAndGet(t *testing.T) {
	reg := NewInMemoryAgentRegistry()
	agent := NewGenericAgentStub(testConfig())
	reg.Add(agent)

	got, ok := reg.Get("test-agent")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.ID() != "test-agent" {
		t.Errorf("ID = %q", got.ID())
	}
}

func TestInMemoryAgentRegistry_GetMissing(t *testing.T) {
	reg := NewInMemoryAgentRegistry()
	_, ok := reg.Get("nope")
	if ok {
		t.Error("Get should return false for missing agent")
	}
}

func TestInMemoryAgentRegistry_Remove(t *testing.T) {
	reg := NewInMemoryAgentRegistry()
	agent := NewGenericAgentStub(testConfig())
	reg.Add(agent)
	reg.Remove("test-agent")

	_, ok := reg.Get("test-agent")
	if ok {
		t.Error("Get should return false after Remove")
	}
}

func TestInMemoryAgentRegistry_List(t *testing.T) {
	reg := NewInMemoryAgentRegistry()
	if len(reg.List()) != 0 {
		t.Error("empty registry should return empty list")
	}

	for i := range 3 {
		cfg := testConfig()
		cfg.ID = fmt.Sprintf("agent-%d", i)
		reg.Add(NewGenericAgentStub(cfg))
	}

	if len(reg.List()) != 3 {
		t.Errorf("List() len = %d, want 3", len(reg.List()))
	}
}

func TestInMemoryAgentRegistry_FindIdle_Found(t *testing.T) {
	reg := NewInMemoryAgentRegistry()
	reg.Add(NewGenericAgentStub(testConfig()))

	task := Task{ID: "t1", Requirements: []Capability{{Type: "filesystem", Resource: "workspace"}}}
	agent, ok := reg.FindIdle(task)
	if !ok {
		t.Fatal("FindIdle should find an agent")
	}
	if agent.ID() != "test-agent" {
		t.Errorf("ID = %q", agent.ID())
	}
}

func TestInMemoryAgentRegistry_FindIdle_NotFound(t *testing.T) {
	reg := NewInMemoryAgentRegistry()
	reg.Add(NewGenericAgentStub(testConfig()))

	task := Task{ID: "t1", Requirements: []Capability{{Type: "vision"}}}
	_, ok := reg.FindIdle(task)
	if ok {
		t.Error("FindIdle should return false when no agent matches")
	}
}

func TestInMemoryAgentRegistry_FindIdle_EmptyRegistry(t *testing.T) {
	reg := NewInMemoryAgentRegistry()
	_, ok := reg.FindIdle(Task{ID: "t1"})
	if ok {
		t.Error("FindIdle should return false on empty registry")
	}
}
