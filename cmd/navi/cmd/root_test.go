package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"navi/cmd/navi/cmd"
	"navi/internal/core/domain"
	"navi/internal/core/services/chat"
)

// stubLLM satisfies ports.LLMPort for wiring a real chat.Service in tests.
type stubLLM struct {
	reply string
	err   error
}

func (s *stubLLM) Chat(_ context.Context, _ []domain.Message) (string, error) {
	return s.reply, s.err
}

func newDeps(reply string, err error) cmd.Dependencies {
	return cmd.Dependencies{
		Chat: chat.New(&stubLLM{reply: reply, err: err}),
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

func TestServe_Runs(t *testing.T) {
	out, err := execute(newDeps("", nil), "serve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "navi server") {
		t.Errorf("output %q should mention 'navi server'", out)
	}
}

func TestServe_CustomPort(t *testing.T) {
	out, err := execute(newDeps("", nil), "serve", "--port", "9090")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "9090") {
		t.Errorf("output %q should contain port 9090", out)
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
