package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"navi/internal/adapters/llm/openai"
	"navi/internal/core/domain"
)

type mockChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func newMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func successHandler(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := mockChatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{Content: content},
					FinishReason: "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func TestNew_ValidatesAPIKey(t *testing.T) {
	_, err := openai.New("", "gpt-4o", "", 0.7, 4096)
	require.Error(t, err)
	require.Contains(t, err.Error(), "api key is required")
}

func TestNew_ValidatesModel(t *testing.T) {
	_, err := openai.New("test-key", "", "", 0.7, 4096)
	require.Error(t, err)
	require.Contains(t, err.Error(), "model is required")
}

func TestNew_WithOptions(t *testing.T) {
	adapter, err := openai.New("test-key", "gpt-4o", "http://localhost:8080", 0.5, 100, openai.WithTimeout(30*time.Second))
	require.NoError(t, err)
	require.NotNil(t, adapter)
}

func TestNewFromConfig_Success(t *testing.T) {
	srv := newMockServer(t, successHandler("Hello from config!"))

	cfg := domain.AgentConfig{
		LLMAPIKey:      "test-key",
		LLMModel:       "gpt-4o",
		LLMBaseURL:     srv.URL,
		LLMTemperature: 0.5,
		LLMMaxTokens:   100,
	}

	adapter, err := openai.NewFromConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, adapter)

	result, err := adapter.Generate(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, "Hello from config!", result)
}

func TestNewFromConfig_MissingAPIKey_UsesEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-api-key")
	srv := newMockServer(t, successHandler("Hello from env!"))

	cfg := domain.AgentConfig{
		LLMModel:   "gpt-4o",
		LLMBaseURL: srv.URL,
	}

	adapter, err := openai.NewFromConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, adapter)

	result, err := adapter.Generate(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, "Hello from env!", result)
}

func TestNewFromConfig_WithTimeout(t *testing.T) {
	srv := newMockServer(t, successHandler("ok"))

	cfg := domain.AgentConfig{
		LLMAPIKey:  "test-key",
		LLMModel:   "gpt-4o",
		LLMBaseURL: srv.URL,
		Timeout:    5 * time.Second,
	}

	adapter, err := openai.NewFromConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, adapter)
}

func TestGenerate_Success(t *testing.T) {
	srv := newMockServer(t, successHandler("Hello from mock!"))

	adapter, err := openai.New("test-key", "test-model", srv.URL, 0.5, 100)
	require.NoError(t, err)

	result, err := adapter.Generate(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, "Hello from mock!", result)
}

func TestGenerate_SendsCorrectHeaders(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer my-key", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/chat/completions", r.URL.Path)

		successHandler("ok")(w, r)
	})

	adapter, err := openai.New("my-key", "gpt-4o", srv.URL, 0.7, 4096)
	require.NoError(t, err)

	_, err = adapter.Generate(context.Background(), "test")
	require.NoError(t, err)
}

func TestGenerate_SendsCorrectBody(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model       string  `json:"model"`
			Temperature float64 `json:"temperature"`
			MaxTokens   int     `json:"max_tokens"`
			Stream      bool    `json:"stream"`
			Messages    []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		require.Equal(t, "my-model", body.Model)
		require.InDelta(t, 0.3, body.Temperature, 0.001)
		require.Equal(t, 512, body.MaxTokens)
		require.False(t, body.Stream)
		require.Len(t, body.Messages, 1)
		require.Equal(t, "user", body.Messages[0].Role)
		require.Equal(t, "my prompt", body.Messages[0].Content)

		successHandler("ok")(w, r)
	})

	adapter, err := openai.New("k", "my-model", srv.URL, 0.3, 512)
	require.NoError(t, err)

	_, err = adapter.Generate(context.Background(), "my prompt")
	require.NoError(t, err)
}

func TestGenerate_APIError(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"error": map[string]string{
				"message": "rate limit exceeded",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	adapter, err := openai.New("k", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	_, err = adapter.Generate(context.Background(), "hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "rate limit exceeded")
}

func TestGenerate_EmptyChoices(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{"choices": []any{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	adapter, err := openai.New("k", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	_, err = adapter.Generate(context.Background(), "hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no choices returned")
}

func TestGenerate_InvalidJSON(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json"))
	})

	adapter, err := openai.New("k", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	_, err = adapter.Generate(context.Background(), "hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse response")
}

func TestGenerate_HTTPError(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"choices":[]}`))
	})

	adapter, err := openai.New("k", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	_, err = adapter.Generate(context.Background(), "hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 500")
}

func TestGenerate_CancelledContext(t *testing.T) {
	srv := newMockServer(t, successHandler("ok"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	adapter, err := openai.New("k", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	_, err = adapter.Generate(ctx, "hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "do request")
}

func TestStream_RealStreaming(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify stream: true is sent
		var body struct {
			Stream bool `json:"stream"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		require.True(t, body.Stream)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Simulate streaming response
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" World\"}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	})

	adapter, err := openai.New("k", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	var received string
	err = adapter.Stream(context.Background(), "hello", func(s string) {
		received += s
	})
	require.NoError(t, err)
	require.Equal(t, "Hello World", received)
}

func TestStream_SingleChunk(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"single\"}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	})

	adapter, err := openai.New("k", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	var received string
	err = adapter.Stream(context.Background(), "test", func(s string) {
		received += s
	})
	require.NoError(t, err)
	require.Equal(t, "single", received)
}

func TestStream_ErrorResponse(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Send error in stream chunk - current implementation skips error chunks
		// This test documents current behavior (ignores error chunks)
		w.Write([]byte("data: {\"error\":{\"message\":\"api error\"}}\n\n"))
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	})

	adapter, err := openai.New("k", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	// Current implementation skips error chunks, so this should succeed
	var received string
	err = adapter.Stream(context.Background(), "test", func(s string) {
		received += s
	})
	require.NoError(t, err)
	require.Equal(t, "ok", received)
}

func TestStream_HTTPError(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	})

	adapter, err := openai.New("k", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	err = adapter.Stream(context.Background(), "test", func(string) {})
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 500")
}

func TestHealth_Success(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		require.Equal(t, "Bearer my-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	})

	adapter, err := openai.New("my-key", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	err = adapter.Health(context.Background())
	require.NoError(t, err)
}

func TestHealth_Failure(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	adapter, err := openai.New("bad-key", "m", srv.URL, 0.7, 100)
	require.NoError(t, err)

	err = adapter.Health(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "health check status 401")
}

func TestNew_Defaults(t *testing.T) {
	adapter, err := openai.New("key", "model", "", 0, 0)
	require.NoError(t, err)
	require.NotNil(t, adapter)

	// The adapter should use defaults. We verify by calling Health against the default URL
	// which will fail with a network error (not a nil panic).
	err = adapter.Health(context.Background())
	require.Error(t, err)
}
