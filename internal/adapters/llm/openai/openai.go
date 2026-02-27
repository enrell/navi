// Package openai provides an LLMPort adapter for the OpenAI REST API.
// It uses only the standard library (net/http) — no external SDK required.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.openai.com/v1"

// OpenAIAdapter implements ports.LLMPort using the OpenAI Chat Completions API.
type OpenAIAdapter struct {
	apiKey      string
	model       string
	baseURL     string
	temperature float64
	maxTokens   int
	client      *http.Client
}

// New creates an OpenAIAdapter. If baseURL is empty, the default OpenAI URL is used.
func New(apiKey, model, baseURL string, temperature float64, maxTokens int) *OpenAIAdapter {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if temperature == 0 {
		temperature = 0.7
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}
	return &OpenAIAdapter{
		apiKey:      apiKey,
		model:       model,
		baseURL:     baseURL,
		temperature: temperature,
		maxTokens:   maxTokens,
		client:      &http.Client{Timeout: 120 * time.Second},
	}
}

// ─── Request / Response types ────────────────────────────────────────────────

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
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

// ─── LLMPort implementation ──────────────────────────────────────────────────

// Generate sends a single prompt and returns the model's full response.
func (a *OpenAIAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	reqBody := chatRequest{
		Model: a.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: a.temperature,
		MaxTokens:   a.maxTokens,
		Stream:      false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai: http request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai: read response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("openai: parse response: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("openai API error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices returned")
	}
	return chatResp.Choices[0].Message.Content, nil
}

// Stream sends a prompt and calls chunk for each streamed token.
// TODO: implement SSE parsing for streaming responses.
func (a *OpenAIAdapter) Stream(ctx context.Context, prompt string, chunk func(string)) error {
	// For now, fall back to non-streaming and emit as a single chunk.
	result, err := a.Generate(ctx, prompt)
	if err != nil {
		return err
	}
	chunk(result)
	return nil
}

// Health checks connectivity to the OpenAI API.
func (a *OpenAIAdapter) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("openai: health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openai: health check status %d", resp.StatusCode)
	}
	return nil
}
