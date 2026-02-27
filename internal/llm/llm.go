package llm

import (
	"context"
)

type LLMProvider interface {
	Complete(ctx context.Context, prompt string, options map[string]any) (string, error)
	Stream(ctx context.Context, prompt string, options map[string]any, chunkHandler func(string)) error
	Embed(ctx context.Context, text string) ([]float64, error)
	Health(ctx context.Context) error
}

type OpenAIClient interface {
	LLMProvider
	SetAPIKey(key string)
	SetBaseURL(url string)
	SetModel(model string)
}

type AnthropicClient interface {
	LLMProvider
	SetAPIKey(key string)
	SetVersion(version string)
	SetMaxTokens(tokens int)
}

type OllamaClient interface {
	LLMProvider
	SetHost(host string)
	SetModel(model string)
}
