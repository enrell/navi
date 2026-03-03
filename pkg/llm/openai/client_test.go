package openai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"navi/pkg/llm/openai"
)

func TestChat_Success(t *testing.T) {
	resp := map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": "Hello, world!"}},
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify request structure
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content-type: %s", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})

	got, err := client.Chat(context.Background(), []openai.Message{
		{Role: "user", Content: "hi"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello, world!" {
		t.Errorf("got %q, want %q", got, "Hello, world!")
	}
}

func TestChat_RequestBody(t *testing.T) {
	var body map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer ts.Close()

	client := openai.New(openai.Config{BaseURL: ts.URL, APIKey: "k", Model: "my-model"})
	_, _ = client.Chat(context.Background(), []openai.Message{
		{Role: "system", Content: "you are helpful"},
		{Role: "user", Content: "hello"},
	})

	if body["model"] != "my-model" {
		t.Errorf("model = %v, want my-model", body["model"])
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Errorf("messages len = %d, want 2", len(msgs))
	}
}

func TestChat_RequestBody_OmitsEmptyModel(t *testing.T) {
	var body map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer ts.Close()

	client := openai.New(openai.Config{BaseURL: ts.URL, APIKey: "k", Model: ""})
	_, _ = client.Chat(context.Background(), []openai.Message{{Role: "user", Content: "hello"}})

	if _, ok := body["model"]; ok {
		t.Fatalf("model should be omitted when empty, got body: %+v", body)
	}
}

func TestChat_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Invalid API key",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer ts.Close()

	client := openai.New(openai.Config{BaseURL: ts.URL, APIKey: "bad", Model: "m"})
	_, err := client.Chat(context.Background(), []openai.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid API key") {
		t.Errorf("error %q does not mention 'Invalid API key'", err.Error())
	}
}

func TestChat_EmptyChoices(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer ts.Close()

	client := openai.New(openai.Config{BaseURL: ts.URL, APIKey: "k", Model: "m"})
	_, err := client.Chat(context.Background(), []openai.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("error %q should mention 'no choices'", err.Error())
	}
}

func TestChat_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("this is not json at all"))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{BaseURL: ts.URL, APIKey: "k", Model: "m"})
	_, err := client.Chat(context.Background(), []openai.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestChat_InvalidURL(t *testing.T) {
	// An invalid scheme forces http.NewRequestWithContext to return an error
	// before any network I/O occurs.
	client := openai.New(openai.Config{BaseURL: "://bad-scheme", APIKey: "k", Model: "m"})
	_, err := client.Chat(context.Background(), []openai.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("error %q should mention 'build request'", err.Error())
	}
}

func TestChat_ReadBodyError(t *testing.T) {
	// brokenBodyTransport returns a 200 with a body reader that always errors,
	// triggering the io.ReadAll error branch.
	client := openai.NewWithClient(
		openai.Config{BaseURL: "http://stub", APIKey: "k", Model: "m"},
		&http.Client{Transport: &brokenBodyTransport{}},
	)
	_, err := client.Chat(context.Background(), []openai.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error when body read fails")
	}
	if !strings.Contains(err.Error(), "read body") {
		t.Errorf("error %q should mention 'read body'", err.Error())
	}
}

// brokenBodyTransport returns a valid HTTP 200 whose body always errors on Read.
type brokenBodyTransport struct{}

func (brokenBodyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(&brokenReader{}),
	}, nil
}

type brokenReader struct{}

func (*brokenReader) Read(_ []byte) (int, error) {
	return 0, errors.New("body read error")
}

func TestChat_ContextCanceled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// never respond — context should cancel
		<-r.Context().Done()
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := openai.New(openai.Config{BaseURL: ts.URL, APIKey: "k", Model: "m"})
	_, err := client.Chat(ctx, []openai.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestChat_Integration_NVIDIA calls the real NVIDIA API.
// Skipped unless NVIDIA_API_KEY is set in the environment.
func TestChat_Integration_NVIDIA(t *testing.T) {
	apiKey := os.Getenv("NVIDIA_API_KEY")
	if apiKey == "" {
		t.Skip("NVIDIA_API_KEY not set — skipping integration test")
	}

	client := openai.New(openai.Config{
		BaseURL: "https://integrate.api.nvidia.com/v1",
		APIKey:  apiKey,
		Model:   "meta/llama-3.1-8b-instruct",
	})

	got, err := client.Chat(context.Background(), []openai.Message{
		{Role: "user", Content: "Reply with exactly the word: PONG"},
	})
	if err != nil {
		t.Fatalf("integration error: %v", err)
	}
	if got == "" {
		t.Error("empty response from NVIDIA API")
	}
	t.Logf("NVIDIA response: %s", got)
}
