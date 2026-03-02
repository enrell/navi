package chat_test

import (
	"context"
	"errors"
	"testing"

	"navi/internal/core/domain"
	"navi/internal/core/services/chat"
)

// stubLLM is a test double that implements ports.LLMPort without any HTTP.
type stubLLM struct {
	reply string
	err   error
	// captured holds the messages the stub received for assertion.
	captured []domain.Message
}

func (s *stubLLM) Chat(_ context.Context, messages []domain.Message) (string, error) {
	s.captured = messages
	return s.reply, s.err
}

func TestChat_SendsUserMessage(t *testing.T) {
	stub := &stubLLM{reply: "Hello back!"}
	svc := chat.New(stub)

	got, err := svc.Chat(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello back!" {
		t.Errorf("got %q, want %q", got, "Hello back!")
	}
	if len(stub.captured) != 1 {
		t.Fatalf("expected 1 message, got %d", len(stub.captured))
	}
	if stub.captured[0].Role != domain.RoleUser {
		t.Errorf("role = %q, want %q", stub.captured[0].Role, domain.RoleUser)
	}
	if stub.captured[0].Content != "Hello" {
		t.Errorf("content = %q, want %q", stub.captured[0].Content, "Hello")
	}
}

func TestChat_EmptyMessage(t *testing.T) {
	svc := chat.New(&stubLLM{})
	_, err := svc.Chat(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}

func TestChat_PropagatesLLMError(t *testing.T) {
	stub := &stubLLM{err: errors.New("provider down")}
	svc := chat.New(stub)

	_, err := svc.Chat(context.Background(), "hi")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !errors.Is(err, stub.err) {
		t.Errorf("got %v, want wrapped %v", err, stub.err)
	}
}
