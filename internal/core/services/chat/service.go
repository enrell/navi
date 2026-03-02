// Package chat implements the chat use case.
//
// It is the only place that orchestrates a single-turn conversation:
//   - it builds the message list from a plain string
//   - it delegates generation to the LLMPort
//
// No HTTP, no config files, no CLI flags live here.
package chat

import (
	"context"
	"fmt"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
)

// Service executes the chat use case.
type Service struct {
	llm ports.LLMPort
}

// New returns a ready-to-use Service.
// The caller (main / wiring layer) is responsible for providing the LLMPort.
func New(llm ports.LLMPort) *Service {
	return &Service{llm: llm}
}

// Chat sends a single user message and returns the assistant reply.
func (s *Service) Chat(ctx context.Context, userMessage string) (string, error) {
	if userMessage == "" {
		return "", fmt.Errorf("chat: message cannot be empty")
	}

	messages := []domain.Message{
		{Role: domain.RoleUser, Content: userMessage},
	}

	reply, err := s.llm.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}

	return reply, nil
}
