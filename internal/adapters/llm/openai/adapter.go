// Package openai adapts the generic OpenAI HTTP client to the ports.LLMPort
// interface required by the core services.
//
// Dependency direction:
//
//	core/ports.LLMPort  ←  adapters/llm/openai.Adapter  →  pkg/llm/openai.Client
//
// The core never imports this package — it only speaks the port interface.
package openai

import (
	"context"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
	pkgopenai "navi/pkg/llm/openai"
)

// Adapter wraps the generic HTTP client and satisfies ports.LLMPort.
type Adapter struct {
	client *pkgopenai.Client
}

// New creates an Adapter from the given openai.Config.
// Use pkg/llm provider presets (llm.NVIDIA, llm.Groq, …) to build the config.
func New(cfg pkgopenai.Config) *Adapter {
	return &Adapter{client: pkgopenai.New(cfg)}
}

// Compile-time check: Adapter must implement ports.LLMPort.
var _ ports.LLMPort = (*Adapter)(nil)

// Chat translates the domain messages to the HTTP layer and back.
func (a *Adapter) Chat(ctx context.Context, messages []domain.Message) (string, error) {
	httpMsgs := make([]pkgopenai.Message, len(messages))
	for i, m := range messages {
		httpMsgs[i] = pkgopenai.Message{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}
	return a.client.Chat(ctx, httpMsgs)
}
