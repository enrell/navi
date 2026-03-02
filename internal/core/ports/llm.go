// Package ports declares the outbound interfaces (secondary ports) that the
// core services depend on. Adapters in internal/adapters/ implement them.
package ports

import (
	"context"

	"navi/internal/core/domain"
)

// LLMPort is the interface every LLM adapter must satisfy.
// The core never imports any concrete adapter — it only knows this contract.
type LLMPort interface {
	// Chat sends a sequence of messages and returns the assistant reply.
	Chat(ctx context.Context, messages []domain.Message) (string, error)
}
