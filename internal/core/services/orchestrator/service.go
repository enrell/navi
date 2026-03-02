package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
)

const defaultMaxTurns = 4

type toolCall struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

// Service is a simple orchestrator agent that can call tools in a loop.
type Service struct {
	llm      ports.LLMPort
	tools    ports.ToolExecutor
	maxTurns int
}

func New(llm ports.LLMPort, tools ports.ToolExecutor) *Service {
	return &Service{llm: llm, tools: tools, maxTurns: defaultMaxTurns}
}

// Ask runs a short plan/act loop.
// The model can request a tool by replying with:
// TOOL_CALL {"name":"tool.name","input":"..."}
func (s *Service) Ask(ctx context.Context, userMessage string) (string, error) {
	if strings.TrimSpace(userMessage) == "" {
		return "", fmt.Errorf("orchestrator: message cannot be empty")
	}

	systemPrompt, err := s.buildSystemPrompt(ctx)
	if err != nil {
		return "", err
	}

	messages := []domain.Message{
		{Role: domain.RoleSystem, Content: systemPrompt},
		{Role: domain.RoleUser, Content: userMessage},
	}

	for i := 0; i < s.maxTurns; i++ {
		reply, err := s.llm.Chat(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("orchestrator: llm: %w", err)
		}

		call, ok := parseToolCall(reply)
		if !ok {
			return strings.TrimSpace(reply), nil
		}

		result, err := s.tools.ExecuteTool(ctx, call.Name, call.Input)
		if err != nil {
			result = "tool execution error: " + err.Error()
		}

		messages = append(messages,
			domain.Message{Role: domain.RoleAssistant, Content: strings.TrimSpace(reply)},
			domain.Message{Role: domain.RoleUser, Content: fmt.Sprintf("TOOL_RESULT name=%s\n%s", call.Name, result)},
		)
	}

	return "", fmt.Errorf("orchestrator: max tool iterations reached")
}

func (s *Service) buildSystemPrompt(ctx context.Context) (string, error) {
	tools, err := s.tools.ListTools(ctx)
	if err != nil {
		return "", fmt.Errorf("orchestrator: list tools: %w", err)
	}

	lines := []string{
		"You are Navi orchestrator, a practical assistant.",
		"If a tool is needed, reply with exactly one line:",
		"TOOL_CALL {\"name\":\"tool.name\",\"input\":\"text input\"}",
		"If no tool is needed, answer normally.",
		"Available tools:",
	}

	names := make([]string, 0, len(tools))
	byName := make(map[string]string, len(tools))
	for _, tool := range tools {
		n := strings.TrimSpace(tool.Name)
		if n == "" {
			continue
		}
		names = append(names, n)
		byName[n] = strings.TrimSpace(tool.Description)
	}
	sort.Strings(names)

	for _, name := range names {
		desc := byName[name]
		if desc == "" {
			desc = "no description"
		}
		lines = append(lines, "- "+name+": "+desc)
	}

	return strings.Join(lines, "\n"), nil
}

func parseToolCall(reply string) (toolCall, bool) {
	for _, line := range strings.Split(reply, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "TOOL_CALL") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "TOOL_CALL"))
		if payload == "" {
			return toolCall{}, false
		}

		var call toolCall
		if err := json.Unmarshal([]byte(payload), &call); err != nil {
			return toolCall{}, false
		}
		call.Name = strings.TrimSpace(call.Name)
		if call.Name == "" {
			return toolCall{}, false
		}
		return call, true
	}

	return toolCall{}, false
}
