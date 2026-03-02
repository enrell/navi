package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"navi/cmd/navi/cmd"
	"navi/internal/adapters/persistence/memory"
	"navi/internal/core/domain"
	agentsvc "navi/internal/core/services/agent"
	"navi/internal/core/services/chat"
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
		Chat:   chatService,
		Tasks:  taskService,
		Agents: agentService,
	}
}

func execute(deps cmd.Dependencies, args ...string) (string, error) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(deps, &buf)
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
