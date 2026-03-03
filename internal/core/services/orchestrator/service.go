package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
	"navi/internal/telemetry"
)

const defaultMaxTurns = 4

type toolCall struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

type TraceEventType string

const (
	TraceThinking     TraceEventType = "thinking"
	TraceToolResponse TraceEventType = "tool_response"
	TraceOrchestrator TraceEventType = "orchestrator"
)

type TraceEvent struct {
	Type    TraceEventType
	Tool    string
	Content string
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
	reply, _, err := s.AskWithTrace(ctx, userMessage)
	return reply, err
}

// AskWithTrace runs the same loop as Ask and also returns trace events that the
// TUI can render to make model/tool behavior explicit.
func (s *Service) AskWithTrace(ctx context.Context, userMessage string) (string, []TraceEvent, error) {
	ctx, traceID := telemetry.EnsureTraceID(ctx)
	logger := telemetry.Logger().With("component", "orchestrator", "trace_id", traceID)
	logger.Info("ask_start", "input_chars", len(userMessage))

	if strings.TrimSpace(userMessage) == "" {
		logger.Error("ask_invalid_input")
		return "", nil, fmt.Errorf("orchestrator: message cannot be empty")
	}

	systemPrompt, err := s.buildSystemPrompt(ctx)
	if err != nil {
		logger.Error("build_system_prompt_failed", "error", err.Error())
		return "", nil, err
	}

	trace := make([]TraceEvent, 0, 8)

	messages := []domain.Message{
		{Role: domain.RoleSystem, Content: systemPrompt},
		{Role: domain.RoleUser, Content: userMessage},
	}

	for i := 0; i < s.maxTurns; i++ {
		logger.Info("llm_turn_start", "turn", i+1)
		reply, err := s.llm.Chat(ctx, messages)
		if err != nil {
			logger.Error("llm_turn_failed", "turn", i+1, "error", err.Error())
			return "", trace, fmt.Errorf("orchestrator: llm: %w", err)
		}
		logger.Info("llm_turn_reply", "turn", i+1, "reply_chars", len(reply))

		calls, thinking, ok := parseToolCalls(reply)
		if !ok {
			finalReply := strings.TrimSpace(reply)
			if finalReply != "" {
				trace = append(trace, TraceEvent{Type: TraceOrchestrator, Content: finalReply})
			}
			logger.Info("ask_completed", "turn", i+1, "tool_calls", 0, "reply_chars", len(finalReply))
			return finalReply, trace, nil
		}

		if thinking == "" {
			thinking = "Model requested tool execution."
		}
		trace = append(trace, TraceEvent{Type: TraceThinking, Content: thinking})

		toolResults := make([]string, 0, len(calls))
		for _, call := range calls {
			logger.Info("tool_call_requested", "tool", call.Name, "input_chars", len(call.Input))
			result, err := s.tools.ExecuteTool(ctx, call.Name, call.Input)
			if err != nil {
				logger.Error("tool_call_failed", "tool", call.Name, "error", err.Error())
				result = "tool execution error: " + err.Error()
			}
			logger.Info("tool_call_result", "tool", call.Name, "result_chars", len(result))

			trace = append(trace, TraceEvent{Type: TraceToolResponse, Tool: call.Name, Content: result})
			toolResults = append(toolResults, fmt.Sprintf("TOOL_RESULT name=%s\n%s", call.Name, result))
		}

		toolResultBlock := strings.Join(toolResults, "\n\n")

		messages = append(messages,
			domain.Message{Role: domain.RoleAssistant, Content: strings.TrimSpace(reply)},
			domain.Message{Role: domain.RoleUser, Content: toolResultBlock},
		)
	}

	logger.Error("ask_max_turns_reached", "max_turns", s.maxTurns)
	return "", trace, fmt.Errorf("orchestrator: max tool iterations reached")
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

func parseToolCalls(reply string) ([]toolCall, string, bool) {
	idx := strings.Index(reply, "TOOL_CALL")
	if idx < 0 {
		return nil, "", false
	}

	thinking := strings.TrimSpace(reply[:idx])
	payload := strings.TrimSpace(reply[idx+len("TOOL_CALL"):])
	if payload == "" {
		return nil, thinking, false
	}

	var many []toolCall
	if err := json.Unmarshal([]byte(payload), &many); err == nil {
		cleaned := make([]toolCall, 0, len(many))
		for _, call := range many {
			call.Name = strings.TrimSpace(call.Name)
			if call.Name == "" {
				continue
			}
			cleaned = append(cleaned, call)
		}
		if len(cleaned) == 0 {
			return nil, thinking, false
		}
		return cleaned, thinking, true
	}

	var one toolCall
	if err := json.Unmarshal([]byte(payload), &one); err != nil {
		return nil, thinking, false
	}
	one.Name = strings.TrimSpace(one.Name)
	if one.Name == "" {
		return nil, thinking, false
	}
	return []toolCall{one}, thinking, true
}
