package orchestrator_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
	"navi/internal/core/services/orchestrator"
)

type llmStub struct {
	replies []string
	err     error
	idx     int
	seen    []domain.Message
}

func (s *llmStub) Chat(_ context.Context, messages []domain.Message) (string, error) {
	s.seen = append(s.seen, messages...)
	if s.err != nil {
		return "", s.err
	}
	if s.idx >= len(s.replies) {
		return "", nil
	}
	r := s.replies[s.idx]
	s.idx++
	return r, nil
}

func TestBuildSystemPrompt_IncludesAvailableAgents(t *testing.T) {
	llm := &llmStub{replies: []string{"final"}}
	tools := &toolExecStub{tools: []ports.Tool{{Name: "agent.call", Description: "delegate"}}}
	svc := orchestrator.New(llm, tools)
	svc.SetAvailableAgents([]string{"coder", "tester", "coder"})

	_, err := svc.Ask(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Ask error: %v", err)
	}

	if len(llm.seen) == 0 {
		t.Fatal("expected llm messages to be captured")
	}
	if llm.seen[0].Role != domain.RoleSystem {
		t.Fatalf("first message role = %q, want system", llm.seen[0].Role)
	}
	content := llm.seen[0].Content
	if !(strings.Contains(content, "Available specialist agents") && strings.Contains(content, "- coder") && strings.Contains(content, "- tester")) {
		t.Fatalf("system prompt missing agent list: %q", content)
	}
}

type toolExecStub struct {
	tools    []ports.Tool
	result   string
	err      error
	lastName string
	lastIn   string
	calls    []string
}

func (s *toolExecStub) ListTools(context.Context) ([]ports.Tool, error) {
	return s.tools, nil
}

func (s *toolExecStub) ExecuteTool(_ context.Context, name, input string) (string, error) {
	s.lastName = name
	s.lastIn = input
	s.calls = append(s.calls, name+"="+input)
	if s.err != nil {
		return "", s.err
	}
	return s.result, nil
}

func TestAsk_FinalWithoutToolCall(t *testing.T) {
	llm := &llmStub{replies: []string{"Hello from orchestrator"}}
	tools := &toolExecStub{tools: []ports.Tool{{Name: "native.echo", Description: "Echo input"}}}
	svc := orchestrator.New(llm, tools)

	got, err := svc.Ask(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Ask error: %v", err)
	}
	if got != "Hello from orchestrator" {
		t.Errorf("got %q, want %q", got, "Hello from orchestrator")
	}
}

func TestAsk_ToolCallThenFinal(t *testing.T) {
	llm := &llmStub{replies: []string{
		"TOOL_CALL {\"name\":\"native.echo\",\"input\":\"ping\"}",
		"Tool says ping",
	}}
	tools := &toolExecStub{tools: []ports.Tool{{Name: "native.echo"}}, result: "ping"}
	svc := orchestrator.New(llm, tools)

	got, err := svc.Ask(context.Background(), "use tool")
	if err != nil {
		t.Fatalf("Ask error: %v", err)
	}
	if got != "Tool says ping" {
		t.Errorf("got %q, want %q", got, "Tool says ping")
	}
	if tools.lastName != "native.echo" || tools.lastIn != "ping" {
		t.Errorf("tool call = (%q,%q), want (native.echo,ping)", tools.lastName, tools.lastIn)
	}
}

func TestAsk_PropagatesLLMError(t *testing.T) {
	boom := errors.New("provider down")
	llm := &llmStub{err: boom}
	tools := &toolExecStub{}
	svc := orchestrator.New(llm, tools)

	_, err := svc.Ask(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want wrapped %v", err, boom)
	}
}

func TestAskWithTrace_MultiToolArray(t *testing.T) {
	llm := &llmStub{replies: []string{
		"Let me run all tools.\nTOOL_CALL [{\"name\":\"mcp.echo\",\"input\":\"A\"},{\"name\":\"native.echo\",\"input\":\"B\"}]",
		"All tools completed",
	}}
	tools := &toolExecStub{tools: []ports.Tool{{Name: "mcp.echo"}, {Name: "native.echo"}}, result: "ok"}
	svc := orchestrator.New(llm, tools)

	got, trace, err := svc.AskWithTrace(context.Background(), "test tools")
	if err != nil {
		t.Fatalf("AskWithTrace error: %v", err)
	}
	if got != "All tools completed" {
		t.Errorf("got %q, want %q", got, "All tools completed")
	}
	if len(tools.calls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(tools.calls))
	}
	if tools.calls[0] != "mcp.echo=A" || tools.calls[1] != "native.echo=B" {
		t.Errorf("tool calls = %+v, want [mcp.echo=A native.echo=B]", tools.calls)
	}

	if len(trace) < 4 {
		t.Fatalf("trace len = %d, want at least 4", len(trace))
	}
	if trace[0].Type != orchestrator.TraceThinking {
		t.Fatalf("trace[0].Type = %q, want thinking", trace[0].Type)
	}
	if trace[1].Type != orchestrator.TraceToolResponse || trace[2].Type != orchestrator.TraceToolResponse {
		t.Fatalf("expected tool response events in trace: %+v", trace)
	}
	if trace[len(trace)-1].Type != orchestrator.TraceOrchestrator {
		t.Fatalf("last trace type = %q, want orchestrator", trace[len(trace)-1].Type)
	}
}
