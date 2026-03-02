package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"navi/internal/adapters/llm/openai"
	"navi/internal/core/domain"
	pkgopenai "navi/pkg/llm/openai"
)

func newFakeServer(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": reply}},
			},
		})
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestAdapter_Chat_TranslatesMessages(t *testing.T) {
	var receivedBody struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Model string `json:"model"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	t.Cleanup(ts.Close)

	adapter := openai.New(pkgopenai.Config{
		BaseURL: ts.URL,
		APIKey:  "test",
		Model:   "test-model",
	})

	_, err := adapter.Chat(context.Background(), []domain.Message{
		{Role: domain.RoleSystem, Content: "you are helpful"},
		{Role: domain.RoleUser, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(receivedBody.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(receivedBody.Messages))
	}
	if receivedBody.Messages[0].Role != "system" {
		t.Errorf("role[0] = %q, want system", receivedBody.Messages[0].Role)
	}
	if receivedBody.Messages[1].Role != "user" {
		t.Errorf("role[1] = %q, want user", receivedBody.Messages[1].Role)
	}
	if receivedBody.Model != "test-model" {
		t.Errorf("model = %q, want test-model", receivedBody.Model)
	}
}

func TestAdapter_Chat_ReturnsReply(t *testing.T) {
	ts := newFakeServer(t, "world")
	adapter := openai.New(pkgopenai.Config{
		BaseURL: ts.URL,
		APIKey:  "k",
		Model:   "m",
	})

	got, err := adapter.Chat(context.Background(), []domain.Message{
		{Role: domain.RoleUser, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}
}
