package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tests for the composition root (main.go).
// Command behaviour is tested in cmd/navi/cmd/root_test.go.
// LLM HTTP layer is tested in pkg/llm/openai/client_test.go.
// Adapter translation is tested in internal/adapters/llm/openai/adapter_test.go.
//
// Note: main() itself cannot be unit-tested because it calls os.Exit(1) on
// error — that line is the only legitimately uncoverable statement in the file.

// ── buildLLMConfig ────────────────────────────────────────────────────────────

func TestBuildLLMConfig_MissingKey(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "")
	_, err := buildLLMConfig()
	if err == nil {
		t.Fatal("expected error when NVIDIA_API_KEY is unset")
	}
	if !strings.Contains(err.Error(), "NVIDIA_API_KEY") {
		t.Errorf("error %q should mention NVIDIA_API_KEY", err.Error())
	}
}

func TestBuildLLMConfig_WithKey(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NAVI_LLM_BASE_URL", "")

	cfg, err := buildLLMConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want test-key", cfg.APIKey)
	}
	if !strings.Contains(cfg.BaseURL, "nvidia.com") {
		t.Errorf("BaseURL %q should point to nvidia.com by default", cfg.BaseURL)
	}
}

func TestBuildLLMConfig_BaseURLOverride(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "k")
	t.Setenv("NAVI_LLM_BASE_URL", "http://localhost:9999")

	cfg, err := buildLLMConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "http://localhost:9999" {
		t.Errorf("BaseURL = %q, want http://localhost:9999", cfg.BaseURL)
	}
}

// ── run() ────────────────────────────────────────────────────────────────────

func TestRun_MissingAPIKey(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "")
	err := run([]string{"chat", "hello"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when NVIDIA_API_KEY is unset")
	}
	if !strings.Contains(err.Error(), "NVIDIA_API_KEY") {
		t.Errorf("error %q should mention NVIDIA_API_KEY", err.Error())
	}
}

func TestRun_Help(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "k")
	t.Setenv("NAVI_LLM_BASE_URL", "")

	var buf bytes.Buffer
	// --help exits cleanly (nil error) and prints usage, without calling the LLM.
	err := run([]string{"--help"}, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "navi") {
		t.Errorf("help output %q should mention 'navi'", buf.String())
	}
}

func TestRun_ChatHappyPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "PONG"}},
			},
		})
	}))
	defer ts.Close()

	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NAVI_LLM_BASE_URL", ts.URL)

	var buf bytes.Buffer
	if err := run([]string{"chat", "PING"}, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "PONG") {
		t.Errorf("output %q should contain PONG", buf.String())
	}
}

func TestRun_ChatLLMError(t *testing.T) {
	// Server returns an API error response.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "invalid api key"},
		})
	}))
	defer ts.Close()

	t.Setenv("NVIDIA_API_KEY", "bad-key")
	t.Setenv("NAVI_LLM_BASE_URL", ts.URL)

	err := run([]string{"chat", "hello"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("error %q should mention 'invalid api key'", err.Error())
	}
}
