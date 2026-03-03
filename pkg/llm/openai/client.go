// Package openai provides a generic OpenAI-compatible HTTP client.
//
// All providers that implement the OpenAI chat completions API (Groq, NVIDIA,
// OpenRouter, etc.) share this single client. If OpenAI changes their API,
// updating this file propagates the fix to every provider automatically.
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

// Config holds the parameters needed to reach any OpenAI-compatible endpoint.
type Config struct {
	// BaseURL is the API root, e.g. "https://api.openai.com/v1".
	// In tests, point this at an httptest.Server URL.
	BaseURL string
	// APIKey is sent as a Bearer token in the Authorization header.
	APIKey string
	// Model is the model name forwarded in every request body.
	Model string
}

// Message represents a single turn in a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the JSON body sent to /chat/completions.
type chatRequest struct {
	Model    string    `json:"model,omitempty"`
	Messages []Message `json:"messages"`
}

// chatResponse is the subset of the OpenAI response we care about.
type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    any    `json:"code,omitempty"`
}

// Client is a thin, stateless wrapper around the OpenAI chat completions API.
type Client struct {
	cfg  Config
	http *http.Client
}

// jsonMarshal is a package-level variable so tests can replace it to simulate
// marshal errors. Production code always uses the standard json.Marshal.
var jsonMarshal = json.Marshal

// New creates a Client with a 60-second timeout.
func New(cfg Config) *Client {
	return NewWithClient(cfg, &http.Client{Timeout: 60 * time.Second})
}

// NewWithClient creates a Client with a custom *http.Client.
// Use this in tests to inject a custom transport (broken body, slow response, etc.).
func NewWithClient(cfg Config, httpClient *http.Client) *Client {
	return &Client{
		cfg:  cfg,
		http: httpClient,
	}
}

// Chat sends messages to the configured endpoint and returns the assistant reply.
func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	body, err := jsonMarshal(chatRequest{
		Model:    c.cfg.Model,
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.cfg.BaseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("openai: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai: read body: %w", err)
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("openai: decode response (status %d): %w", resp.StatusCode, err)
	}

	if cr.Error != nil {
		return "", fmt.Errorf("openai: api error: %s", cr.Error.Message)
	}

	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices in response (status %d)", resp.StatusCode)
	}

	return cr.Choices[0].Message.Content, nil
}
