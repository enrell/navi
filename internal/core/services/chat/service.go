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
	"navi/internal/telemetry"
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
	ctx, traceID := telemetry.EnsureTraceID(ctx)
	telemetry.Logger().Info("chat_start", "trace_id", traceID, "input_chars", len(userMessage))
	if userMessage == "" {
		telemetry.Logger().Error("chat_invalid_input", "trace_id", traceID)
		return "", fmt.Errorf("chat: message cannot be empty")
	}

	messages := []domain.Message{
		{Role: domain.RoleUser, Content: userMessage},
	}

	reply, err := s.llm.Chat(ctx, messages)
	if err != nil {
		telemetry.Logger().Error("chat_llm_failed", "trace_id", traceID, "error", err.Error())
		return "", fmt.Errorf("chat: %w", err)
	}
	telemetry.Logger().Info("chat_done", "trace_id", traceID, "reply_chars", len(reply))

	return reply, nil
}
