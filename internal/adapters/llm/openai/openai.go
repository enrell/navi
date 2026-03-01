package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"navi/internal/core/domain"
)

// streamChunk represents a single chunk in OpenAI's streaming response
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

var (
	ErrMissingAPIKey = errors.New("openai: api key is required")
	ErrMissingModel  = errors.New("openai: model is required")
)

const defaultBaseURL = "https://api.openai.com/v1"

type OpenAIAdapter struct {
	apiKey      string
	model       string
	baseURL     string
	temperature float64
	maxTokens   int
	timeout     time.Duration
	client      *http.Client
}

// Option functional option pattern for OpenAIAdapter
type Option func(*OpenAIAdapter)

func WithTimeout(timeout time.Duration) Option {
	return func(a *OpenAIAdapter) {
		a.timeout = timeout
	}
}

func New(apiKey, model, baseURL string, temperature float64, maxTokens int, opts ...Option) (*OpenAIAdapter, error) {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if model == "" {
		return nil, ErrMissingModel
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if temperature == 0 {
		temperature = 0.7
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}

	adapter := &OpenAIAdapter{
		apiKey:      apiKey,
		model:       model,
		baseURL:     baseURL,
		temperature: temperature,
		maxTokens:   maxTokens,
		timeout:     120 * time.Second,
	}

	for _, opt := range opts {
		opt(adapter)
	}

	adapter.client = &http.Client{Timeout: adapter.timeout}

	return adapter, nil
}

// NewFromConfig creates an OpenAIAdapter from AgentConfig following Navi's factory pattern
func NewFromConfig(cfg domain.AgentConfig) (domain.LLMPort, error) {
	apiKey := cfg.LLMAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	opts := []Option{}
	if cfg.Timeout > 0 {
		opts = append(opts, WithTimeout(cfg.Timeout))
	}

	adapter, err := New(apiKey, cfg.LLMModel, cfg.LLMBaseURL, cfg.LLMTemperature, cfg.LLMMaxTokens, opts...)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create adapter from config: %w", err)
	}

	return adapter, nil
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolDef struct {
	Type     string      `json:"type"`
	Function functionDef `json:"function"`
}

type functionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"` // OmitEmpty removed to avoid "content must be provided" errors
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []toolDef     `json:"tools,omitempty"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (a *OpenAIAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := a.Chat(ctx, domain.ChatRequest{
		Messages: []domain.Message{
			{Role: domain.ChatRoleUser, Content: prompt},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Message.Content, nil
}

func (a *OpenAIAdapter) Chat(ctx context.Context, req domain.ChatRequest) (domain.ChatResponse, error) {
	msgs := make([]chatMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				argsBytes, _ := json.Marshal(tc.Arguments)
				msgs[i].ToolCalls = append(msgs[i].ToolCalls, toolCall{
					ID:   tc.RequestID,
					Type: "function",
					Function: functionCall{
						Name:      tc.ToolName,
						Arguments: string(argsBytes),
					},
				})
			}
		}
	}

	var tools []toolDef
	for _, t := range req.Tools {
		tools = append(tools, toolDef{
			Type: "function",
			Function: functionDef{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}

	reqBody := chatRequest{
		Model:       a.model,
		Messages:    msgs,
		Tools:       tools,
		Temperature: a.temperature,
		MaxTokens:   a.maxTokens,
		Stream:      false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("openai: build request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := a.client.Do(httpReq)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("openai: do request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, _ := io.ReadAll(httpResp.Body)

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return domain.ChatResponse{}, fmt.Errorf("openai: HTTP %d, body: %s", httpResp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return domain.ChatResponse{}, fmt.Errorf("openai: parse response: %w (body: %s)", err, string(respBody))
	}
	if chatResp.Error != nil {
		return domain.ChatResponse{}, fmt.Errorf("openai API error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return domain.ChatResponse{}, fmt.Errorf("openai: no choices returned") // we removed the printing of raw body here because it might be too spammy or we just print respBody
	}

	choice := chatResp.Choices[0].Message

	var calls []domain.ToolCall
	for _, tc := range choice.ToolCalls {
		var args map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		// If unmarshal fails it will be empty map, which is fine for error handling further down
		if args == nil {
			args = make(map[string]any)
		}
		calls = append(calls, domain.ToolCall{
			RequestID: tc.ID,
			ToolName:  tc.Function.Name,
			Arguments: args,
		})
	}

	return domain.ChatResponse{
		Message: domain.Message{
			Role:      domain.ChatRole(choice.Role),
			Content:   choice.Content,
			ToolCalls: calls,
		},
	}, nil
}

func (a *OpenAIAdapter) Stream(ctx context.Context, prompt string, chunk func(string)) error {
	reqBody := chatRequest{
		Model: a.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: a.temperature,
		MaxTokens:   a.maxTokens,
		Stream:      true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("openai: marshal streaming request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("openai: build streaming request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := a.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("openai: do streaming request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("openai: streaming HTTP %d, body: %s", httpResp.StatusCode, string(respBody))
	}

	reader := bufio.NewReader(httpResp.Body)
	var content strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("openai: read stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var sc streamChunk
		if err := json.Unmarshal([]byte(data), &sc); err != nil {
			continue // Skip malformed chunks
		}

		if len(sc.Choices) > 0 && sc.Choices[0].Delta.Content != "" {
			content.WriteString(sc.Choices[0].Delta.Content)
			chunk(sc.Choices[0].Delta.Content)
		}
	}

	if content.Len() == 0 {
		return fmt.Errorf("openai: no content in stream")
	}

	return nil
}

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
