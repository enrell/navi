package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"navi/cmd/navi/cmd"
	"navi/internal/adapters/persistence/memory"
	"navi/internal/adapters/tools"
	"navi/internal/core/domain"
	agentsvc "navi/internal/core/services/agent"
	"navi/internal/core/services/chat"
	orchestratorsvc "navi/internal/core/services/orchestrator"
	tasksvc "navi/internal/core/services/task"
)

// stubLLM satisfies ports.LLMPort for wiring a real chat.Service in tests.
type stubLLM struct {
	reply string
	err   error
}

func (s *stubLLM) Chat(_ context.Context, _ []domain.Message) (string, error) {
	return s.reply, s.err
}

// newDeps wires all services so no field in Dependencies is nil.
// Tasks and Agents use in-memory repos so no real I/O occurs.
func newDeps(reply string, err error) cmd.Dependencies {
	llm := &stubLLM{reply: reply, err: err}
	chatService := chat.New(llm)
	taskService := tasksvc.New(memory.NewTaskRepository(), chatService)
	agentService := agentsvc.New(memory.NewAgentRepository(nil))
	return cmd.Dependencies{
		Chat:         chatService,
		Tasks:        taskService,
		Agents:       agentService,
		Orchestrator: nil,
	}
}

func newOrchestratedDeps(replys []string) cmd.Dependencies {
	llm := &sequenceLLM{replies: replys}
	registry := tools.NewRegistry()
	_ = registry.Register("native.echo", "Echo", func(_ context.Context, input string) (string, error) { return input, nil })
	orch := orchestratorsvc.New(llm, registry)

	deps := newDeps("", nil)
	deps.Orchestrator = orch
	return deps
}

func execute(deps cmd.Dependencies, args ...string) (string, error) {
	return executeWithInput(deps, strings.NewReader(""), args...)
}

func executeWithInput(deps cmd.Dependencies, in io.Reader, args ...string) (string, error) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(deps, &buf)
	root.SetIn(in)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// ── chat command ──────────────────────────────────────────────────────────────

func TestChat_PrintsReply(t *testing.T) {
	out, err := execute(newDeps("PONG", nil), "chat", "PING")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "PONG") {
		t.Errorf("output %q should contain PONG", out)
	}
}

func TestChat_MultiWordMessage(t *testing.T) {
	var got string
	llm := &captureLLM{}
	deps := cmd.Dependencies{Chat: chat.New(llm)}
	_, _ = execute(deps, "chat", "hello", "beautiful", "world")
	got = llm.lastContent
	if got != "hello beautiful world" {
		t.Errorf("message = %q, want %q", got, "hello beautiful world")
	}
}

func TestChat_PropagatesError(t *testing.T) {
	_, err := execute(newDeps("", errors.New("llm down")), "chat", "hi")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "llm down") {
		t.Errorf("error %q should mention 'llm down'", err.Error())
	}
}

func TestChat_RequiresArgs(t *testing.T) {
	_, err := execute(newDeps("ok", nil), "chat")
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

// ── repl command ─────────────────────────────────────────────────────────────

func TestRepl_CommandRegistered(t *testing.T) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(newDeps("", nil), &buf)
	replCmd, _, err := root.Find([]string{"repl"})
	if err != nil {
		t.Fatalf("Find repl: %v", err)
	}
	if replCmd.Use != "repl" {
		t.Errorf("Use = %q, want repl", replCmd.Use)
	}
}

func TestTUI_CommandRegistered(t *testing.T) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(newDeps("", nil), &buf)
	tuiCmd, _, err := root.Find([]string{"tui"})
	if err != nil {
		t.Fatalf("Find tui: %v", err)
	}
	if tuiCmd.Use != "tui" {
		t.Errorf("Use = %q, want tui", tuiCmd.Use)
	}
}

func TestRepl_OneMessageThenExit(t *testing.T) {
	in := strings.NewReader("PING\nexit\n")
	out, err := executeWithInput(newDeps("PONG", nil), in, "repl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "PONG") {
		t.Errorf("output %q should contain PONG", out)
	}
	if !strings.Contains(out, "Bye.") {
		t.Errorf("output %q should contain Bye.", out)
	}
}

func TestRepl_ChatErrorPrinted(t *testing.T) {
	in := strings.NewReader("hello\nquit\n")
	out, err := executeWithInput(newDeps("", errors.New("llm down")), in, "repl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "error:") {
		t.Errorf("output %q should contain error prefix", out)
	}
	if !strings.Contains(out, "llm down") {
		t.Errorf("output %q should contain llm down", out)
	}
}

func TestRepl_NilChatService(t *testing.T) {
	deps := newDeps("", nil)
	deps.Chat = nil
	_, err := executeWithInput(deps, strings.NewReader("exit\n"), "repl")
	if err == nil {
		t.Fatal("expected error for missing repl services")
	}
	if !strings.Contains(err.Error(), "neither orchestrator nor chat") {
		t.Errorf("error %q should mention repl service wiring", err.Error())
	}
}

func TestRepl_UsesOrchestratorWhenAvailable(t *testing.T) {
	in := strings.NewReader("hello\nquit\n")
	out, err := executeWithInput(newOrchestratedDeps([]string{"orchestrated response"}), in, "repl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "orchestrated response") {
		t.Errorf("output %q should contain orchestrated response", out)
	}
}

func TestRepl_ClearSectionsForThinkingAndToolResponse(t *testing.T) {
	in := strings.NewReader("hello\nquit\n")
	out, err := executeWithInput(newOrchestratedDeps([]string{
		"Let me run a tool.\nTOOL_CALL {\"name\":\"native.echo\",\"input\":\"ping\"}",
		"Done.",
	}), in, "repl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"User: hello",
		"Thinking:",
		"Tool Response [native.echo]: ping",
		"Orchestrator: Done.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q should contain %q", out, want)
		}
	}
}

func TestRepl_HelpCommand(t *testing.T) {
	in := strings.NewReader(":help\nquit\n")
	out, err := executeWithInput(newDeps("PONG", nil), in, "repl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Commands:", ":help", ":agents"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q should contain %q", out, want)
		}
	}
}

func TestRepl_AgentsCommand(t *testing.T) {
	llm := &stubLLM{reply: "PONG"}
	deps := cmd.Dependencies{
		Chat:   chat.New(llm),
		Agents: agentsvc.New(memory.NewAgentRepository([]*domain.Agent{{ID: "researcher", Name: "Researcher"}})),
	}

	in := strings.NewReader(":agents\nquit\n")
	out, err := executeWithInput(deps, in, "repl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "researcher: Researcher") {
		t.Errorf("output %q should contain agent listing", out)
	}
}

func TestAgents_CommandRegistered(t *testing.T) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(newDeps("", nil), &buf)
	agentsCmd, _, err := root.Find([]string{"agents"})
	if err != nil {
		t.Fatalf("Find agents: %v", err)
	}
	if agentsCmd.Use != "agents" {
		t.Errorf("Use = %q, want agents", agentsCmd.Use)
	}
}

func TestAgents_ListReturnsJSON(t *testing.T) {
	deps := newDeps("", nil)
	deps.Agents = agentsvc.New(memory.NewAgentRepository([]*domain.Agent{{ID: "coder", Name: "Coder"}}))

	out, err := execute(deps, "agents", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"id": "coder"`) {
		t.Errorf("output %q should contain coder agent JSON", out)
	}
}

func TestAgents_GetReturnsJSON(t *testing.T) {
	deps := newDeps("", nil)
	deps.Agents = agentsvc.New(memory.NewAgentRepository([]*domain.Agent{{ID: "tester", Name: "Tester"}}))

	out, err := execute(deps, "agents", "get", "tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"name": "Tester"`) {
		t.Errorf("output %q should contain tester agent JSON", out)
	}
}

func TestTasks_CommandRegistered(t *testing.T) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(newDeps("", nil), &buf)
	tasksCmd, _, err := root.Find([]string{"tasks"})
	if err != nil {
		t.Fatalf("Find tasks: %v", err)
	}
	if tasksCmd.Use != "tasks" {
		t.Errorf("Use = %q, want tasks", tasksCmd.Use)
	}
}

func TestTasks_CreateAndList(t *testing.T) {
	deps := newDeps("done", nil)

	created, err := execute(deps, "tasks", "create", "ship", "it")
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if !strings.Contains(created, `"status": "completed"`) {
		t.Fatalf("create output %q should contain completed status", created)
	}

	listed, err := execute(deps, "tasks", "list")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if !strings.Contains(listed, `"prompt": "ship it"`) {
		t.Errorf("list output %q should contain created task", listed)
	}
}

func TestTasks_GetReturnsCreatedTask(t *testing.T) {
	deps := newDeps("done", nil)

	created, err := execute(deps, "tasks", "create", "inspect", "me")
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	id := extractJSONField(created, `"id":`)
	if id == "" {
		t.Fatalf("could not extract task id from %q", created)
	}

	got, err := execute(deps, "tasks", "get", id)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if !strings.Contains(got, `"prompt": "inspect me"`) {
		t.Errorf("get output %q should contain created task", got)
	}
}

// ── serve command ─────────────────────────────────────────────────────────────
// The serve command starts a real, blocking HTTP server, so we do NOT invoke
// it end-to-end here. Full HTTP handler coverage lives in
// internal/adapters/http/server_test.go.
// These tests verify command registration and flag configuration only.

func TestServe_CommandRegistered(t *testing.T) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(newDeps("", nil), &buf)
	serveCmd, _, err := root.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("Find serve: %v", err)
	}
	if serveCmd.Use != "serve" {
		t.Errorf("Use = %q, want serve", serveCmd.Use)
	}
}

func TestServe_PortFlagDefaultIs8080(t *testing.T) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(newDeps("", nil), &buf)
	serveCmd, _, _ := root.Find([]string{"serve"})
	f := serveCmd.Flags().Lookup("port")
	if f == nil {
		t.Fatal("--port flag not registered")
	}
	if f.DefValue != "8080" {
		t.Errorf("default port = %q, want 8080", f.DefValue)
	}
}

func TestServe_PortFlagShorthand(t *testing.T) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(newDeps("", nil), &buf)
	serveCmd, _, _ := root.Find([]string{"serve"})
	f := serveCmd.Flags().ShorthandLookup("p")
	if f == nil {
		t.Fatal("-p shorthand not registered")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// captureLLM records the content of the last user message.
type captureLLM struct {
	lastContent string
}

func (c *captureLLM) Chat(_ context.Context, messages []domain.Message) (string, error) {
	for _, m := range messages {
		if m.Role == domain.RoleUser {
			c.lastContent = m.Content
		}
	}
	return "ok", nil
}

type sequenceLLM struct {
	replies []string
	i       int
}

func (s *sequenceLLM) Chat(_ context.Context, _ []domain.Message) (string, error) {
	if s.i >= len(s.replies) {
		return "", nil
	}
	r := s.replies[s.i]
	s.i++
	return r, nil
}

func extractJSONField(raw string, prefix string) string {
	idx := strings.Index(raw, prefix)
	if idx < 0 {
		return ""
	}
	rest := raw[idx+len(prefix):]
	firstQuote := strings.Index(rest, `"`)
	if firstQuote < 0 {
		return ""
	}
	rest = rest[firstQuote+1:]
	secondQuote := strings.Index(rest, `"`)
	if secondQuote < 0 {
		return ""
	}
	return rest[:secondQuote]
}
