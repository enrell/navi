// Navi — secure AI orchestrator.
//
// This file is the composition root: it reads environment/config, wires all
// adapters and services together, then hands off to cobra.
//
// Keep it thin. Business logic belongs in internal/core/services/.
// Adapter choices belong in internal/adapters/.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	navcmd "navi/cmd/navi/cmd"
	llmadapter "navi/internal/adapters/llm/openai"
	"navi/internal/adapters/mcp/inprocess"
	"navi/internal/adapters/registry/localfs"
	"navi/internal/adapters/tools"
	"navi/internal/config"
	"navi/internal/core/domain"
	"navi/internal/core/ports"
	"navi/internal/core/services/chat"
	orchestratorsvc "navi/internal/core/services/orchestrator"
	"navi/internal/telemetry"
	llmpkg "navi/pkg/llm"
	pkgopenai "navi/pkg/llm/openai"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run is the testable entry point: it wires all dependencies and executes the
// cobra command tree. Keeping args and out as parameters makes it injectable.
func run(args []string, out io.Writer) error {
	telemetryCloser, err := telemetry.InitDefaultJSONLLogger()
	if err != nil {
		return err
	}
	defer func() {
		_ = telemetryCloser()
	}()
	telemetry.Logger().Info("app_start", "args", strings.Join(args, " "))

	if err := ensureConfigDir(); err != nil {
		telemetry.Logger().Error("ensure_config_dir_failed", "error", err.Error())
		return err
	}
	loadedEnvFiles, err := loadEnvironment()
	if err != nil {
		telemetry.Logger().Error("load_environment_failed", "error", err.Error())
		return err
	}
	if strings.EqualFold(os.Getenv("NAVI_ENV"), "development") && len(loadedEnvFiles) > 0 {
		fmt.Fprintf(out, "[dev] loaded env files: %s\n", strings.Join(loadedEnvFiles, ", "))
	}
	telemetry.Logger().Info("environment_loaded", "navi_env", os.Getenv("NAVI_ENV"), "files", strings.Join(loadedEnvFiles, ","))

	llmCfg, err := buildLLMConfig()
	if err != nil {
		telemetry.Logger().Error("build_llm_config_failed", "error", err.Error())
		return err
	}
	telemetry.Logger().Info("llm_config_resolved", "base_url", llmCfg.BaseURL, "model", llmCfg.Model)

	// Wire: pkg HTTP client → adapter (satisfies LLMPort) → chat service
	adapter := llmadapter.New(llmCfg)
	chatService := chat.New(adapter)

	// Two registries: orchestrator tools (visible to orchestrator prompt) and
	// specialist tools (visible to delegated agents). Read-only tools are
	// intentionally available to both to avoid unnecessary delegation overhead.
	orchestratorTools := tools.NewRegistry()
	specialistTools := tools.NewRegistry()

	_ = orchestratorTools.Register("native.now", "Current UTC time in RFC3339", func(_ context.Context, _ string) (string, error) {
		return time.Now().UTC().Format(time.RFC3339), nil
	})
	_ = orchestratorTools.Register("native.echo", "Echo input text", func(_ context.Context, input string) (string, error) {
		return strings.TrimSpace(input), nil
	})

	// Read-only tools for specialists.
	_ = specialistTools.Register("native.now", "Current UTC time in RFC3339", func(_ context.Context, _ string) (string, error) {
		return time.Now().UTC().Format(time.RFC3339), nil
	})
	_ = specialistTools.Register("native.echo", "Echo input text", func(_ context.Context, input string) (string, error) {
		return strings.TrimSpace(input), nil
	})
	_ = orchestratorTools.Register("native.list_dirs", "List directories in current folder or input path. Input: path string or JSON {\"path\":\".\",\"limit\":10}", func(_ context.Context, input string) (string, error) {
		target, limit, err := parseListDirsInput(input)
		if err != nil {
			return "", fmt.Errorf("native.list_dirs: %w", err)
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return "", fmt.Errorf("native.list_dirs: %w", err)
		}
		dirs := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				dirs = append(dirs, entry.Name())
			}
		}
		sort.Strings(dirs)
		if limit > 0 && limit < len(dirs) {
			dirs = dirs[:limit]
		}
		payload := map[string]any{"path": target, "count": len(dirs), "directories": dirs}
		b, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("native.list_dirs: marshal: %w", err)
		}
		return string(b), nil
	})
	_ = specialistTools.Register("native.list_dirs", "List directories in current folder or input path. Input: path string or JSON {\"path\":\".\",\"limit\":10}", func(_ context.Context, input string) (string, error) {
		target, limit, err := parseListDirsInput(input)
		if err != nil {
			return "", fmt.Errorf("native.list_dirs: %w", err)
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return "", fmt.Errorf("native.list_dirs: %w", err)
		}
		dirs := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				dirs = append(dirs, entry.Name())
			}
		}
		sort.Strings(dirs)
		if limit > 0 && limit < len(dirs) {
			dirs = dirs[:limit]
		}
		payload := map[string]any{"path": target, "count": len(dirs), "directories": dirs}
		b, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("native.list_dirs: marshal: %w", err)
		}
		return string(b), nil
	})

	mcpClient := inprocess.New()
	_ = mcpClient.Register("echo", func(_ context.Context, input string) (string, error) {
		return "mcp echo: " + strings.TrimSpace(input), nil
	})
	if logPath, err := telemetry.LogPath(); err != nil {
		telemetry.Logger().Error("mcp_logs_register_failed", "error", err.Error())
	} else {
		_ = mcpClient.Register("logs", inprocess.NewLogsHandler(logPath))
	}

	mcpEchoHandler := func(ctx context.Context, input string) (string, error) {
		return mcpClient.CallTool(ctx, "echo", input)
	}
	mcpLogsHandler := func(ctx context.Context, input string) (string, error) {
		return mcpClient.CallTool(ctx, "logs", input)
	}

	_ = orchestratorTools.Register("mcp.echo", "Call MCP echo tool", mcpEchoHandler)
	_ = orchestratorTools.Register("mcp.logs", "Query telemetry logs. Input can be JSON {\"limit\":20,\"trace_id\":\"...\",\"level\":\"error\",\"component\":\"orchestrator\",\"contains\":\"text\"}", mcpLogsHandler)
	_ = specialistTools.Register("mcp.echo", "Call MCP echo tool", mcpEchoHandler)
	_ = specialistTools.Register("mcp.logs", "Query telemetry logs. Input can be JSON {\"limit\":20,\"trace_id\":\"...\",\"level\":\"error\",\"component\":\"orchestrator\",\"contains\":\"text\"}", mcpLogsHandler)

	defaultAgents, agentIDs, err := loadDefaultSpecialistAgents()
	if err != nil {
		telemetry.Logger().Error("load_default_agents_failed", "error", err.Error())
	} else {
		telemetry.Logger().Info("load_default_agents_done", "count", len(defaultAgents), "agents", strings.Join(agentIDs, ","))
	}
	if len(defaultAgents) > 0 {
		err := orchestratorTools.Register("agent.call", "Delegate task to a specialist agent. Input JSON: {\"agent_id\":\"coder\",\"prompt\":\"...\"}. Required when user explicitly requests a specific specialist.", buildAgentDelegationTool(adapter, specialistTools, defaultAgents))
		if err != nil {
			telemetry.Logger().Error("register_agent_call_tool_failed", "error", err.Error())
		}
	}

	orchestratorService := orchestratorsvc.New(adapter, orchestratorTools)
	orchestratorService.SetAvailableAgents(agentIDs)

	deps := navcmd.Dependencies{
		Chat:         chatService,
		Tasks:        nil, // serve command lazily wires SQLite-backed task service
		Agents:       nil, // serve command lazily wires SQLite-backed agent service
		Orchestrator: orchestratorService,
		ModelName:    llmCfg.Model,
		WorkDir:      mustGetwd(),
		ContextLimit: inferContextWindow(llmCfg.Model),
	}

	root := navcmd.NewRootCommand(deps, out)
	root.SetArgs(args)
	err = root.Execute()
	if err != nil {
		telemetry.Logger().Error("command_execute_failed", "error", err.Error())
		return err
	}
	telemetry.Logger().Info("app_stop")
	return nil
}

// configPath is a seam for tests that need to simulate config.Path() failures.
var configPath = config.Path
var configDir = config.Dir

func ensureConfigDir() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("config: create dir %q: %w", dir, err)
	}
	return nil
}

func loadEnvironment() ([]string, error) {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("NAVI_ENV")))
	if env == "" {
		env = "development"
		_ = os.Setenv("NAVI_ENV", env)
	}

	paths := []string{".env", ".env.local", ".env." + env, ".env." + env + ".local"}
	existing := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	if len(existing) == 0 {
		return nil, nil
	}
	if err := godotenv.Load(existing...); err != nil {
		return nil, fmt.Errorf("env: load .env files: %w", err)
	}
	return existing, nil
}

// buildLLMConfig loads the user config file and resolves it to a pkgopenai.Config.
//
// Resolution order:
//  1. Read ~/.config/navi/config.toml (or platform equivalent); fall back to
//     defaults if the file does not exist.
//  2. Resolve the API key from the environment variable named in api_key_env.
//  3. Apply NAVI_LLM_BASE_URL env override (used in tests / local proxies).
func buildLLMConfig() (pkgopenai.Config, error) {
	path, err := configPath()
	if err != nil {
		return pkgopenai.Config{}, err
	}
	return buildLLMConfigFrom(path)
}

// buildLLMConfigFrom is the testable variant: it accepts an explicit config path
// so tests can point at a temp file without touching the real user config.
func buildLLMConfigFrom(cfgPath string) (pkgopenai.Config, error) {
	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		return pkgopenai.Config{}, err
	}
	if err := applyEnvironmentOverrides(&cfg); err != nil {
		return pkgopenai.Config{}, err
	}

	apiKey, err := cfg.ResolveAPIKey()
	if err != nil {
		return pkgopenai.Config{}, err
	}

	llmCfg := providerPreset(cfg, apiKey)

	// NAVI_LLM_BASE_URL overrides the provider endpoint — used in tests and
	// local development to point at an httptest server or a local proxy.
	if override := os.Getenv("NAVI_LLM_BASE_URL"); override != "" {
		llmCfg.BaseURL = override
	}

	return llmCfg, nil
}

func applyEnvironmentOverrides(cfg *config.Config) error {
	if provider := strings.TrimSpace(os.Getenv("NAVI_DEFAULT_PROVIDER")); provider != "" {
		provider = strings.ToLower(provider)
		switch provider {
		case config.ProviderNVIDIA, config.ProviderOpenAI, config.ProviderGroq, config.ProviderOpenRouter, config.ProviderOllama:
			cfg.DefaultLLM.Provider = provider
		default:
			return fmt.Errorf("env: invalid NAVI_DEFAULT_PROVIDER %q", provider)
		}
	}

	if model := strings.TrimSpace(os.Getenv("NAVI_DEFAULT_MODEL")); model != "" {
		cfg.DefaultLLM.Model = model
	}

	if apiKeyEnv := strings.TrimSpace(os.Getenv("NAVI_DEFAULT_API_KEY_ENV")); apiKeyEnv != "" {
		cfg.DefaultLLM.APIKeyEnv = apiKeyEnv
	}

	if apiKey := strings.TrimSpace(os.Getenv("NAVI_API_KEY")); apiKey != "" {
		_ = os.Setenv("NAVI_API_KEY", apiKey)
		cfg.DefaultLLM.APIKeyEnv = "NAVI_API_KEY"
	}

	return nil
}

// providerPreset maps a validated Config to the corresponding pkgopenai.Config
// preset, then applies any per-field overrides (model, base_url).
//
// The provider field must already be normalised to lowercase and validated by
// config.LoadFrom — this function trusts that invariant and does not error.
func providerPreset(cfg config.Config, apiKey string) pkgopenai.Config {
	llm := cfg.DefaultLLM
	var preset pkgopenai.Config

	switch llm.Provider {
	case config.ProviderOpenAI:
		preset = llmpkg.OpenAI(apiKey)
	case config.ProviderGroq:
		preset = llmpkg.Groq(apiKey)
	case config.ProviderOpenRouter:
		preset = llmpkg.OpenRouter(apiKey)
	case config.ProviderOllama:
		preset = llmpkg.Ollama(llm.Model)
	default: // config.ProviderNVIDIA and "" (empty = NVIDIA)
		preset = llmpkg.NVIDIA(apiKey)
	}

	// Per-field overrides from config file.
	if llm.Model != "" {
		preset.Model = llm.Model
	}
	if llm.BaseURL != "" {
		preset.BaseURL = llm.BaseURL
	}

	return preset
}

func defaultAgentRoots() ([]string, error) {
	baseDir, err := config.Dir()
	if err != nil {
		return nil, err
	}

	roots := []string{filepath.Join(baseDir, "agents")}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Join(wd, "configs", "agents"))
	}
	return roots, nil
}

func loadDefaultSpecialistAgents() (map[string]*domain.GenericAgent, []string, error) {
	roots, err := defaultAgentRoots()
	if err != nil {
		return nil, nil, err
	}

	loaded, err := localfs.LoadGenericAgentsFromRoots(roots)
	if err != nil {
		return nil, nil, err
	}

	byID := make(map[string]*domain.GenericAgent, len(loaded))
	ids := make([]string, 0, len(loaded))
	for _, ga := range loaded {
		if ga == nil {
			continue
		}
		id := ga.ID()
		if id == "" {
			continue
		}
		if !isSpecialistAgentID(id) {
			continue
		}
		byID[id] = ga
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return byID, ids, nil
}

func isSpecialistAgentID(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "planner", "researcher", "coder", "tester":
		return true
	default:
		return false
	}
}

type agentToolInput struct {
	AgentID string `json:"agent_id"`
	Prompt  string `json:"prompt"`
	Task    string `json:"task"`
	Message string `json:"message"`
}

const delegatedAgentMaxTurns = 4

func buildAgentDelegationTool(llm ports.LLMPort, toolExec ports.ToolExecutor, agents map[string]*domain.GenericAgent) tools.Handler {
	return func(ctx context.Context, input string) (string, error) {
		traceID := telemetry.TraceID(ctx)
		agentID, prompt, err := parseAgentCallInput(input)
		if err != nil {
			telemetry.Logger().Error("agent_call_input_invalid", "trace_id", traceID, "error", err.Error())
			return "", err
		}

		agent, ok := agents[agentID]
		if !ok {
			telemetry.Logger().Error("agent_call_unknown_agent", "trace_id", traceID, "agent_id", agentID)
			return "", fmt.Errorf("agent.call: unknown agent %q", agentID)
		}

		messages, err := buildDelegatedAgentMessages(ctx, agent, prompt, toolExec)
		if err != nil {
			telemetry.Logger().Error("agent_call_build_messages_failed", "trace_id", traceID, "agent_id", agentID, "error", err.Error())
			return "", fmt.Errorf("agent.call: %w", err)
		}
		telemetry.Logger().Info("agent_call_start", "trace_id", traceID, "agent_id", agentID, "prompt_chars", len(prompt))

		for turn := 0; turn < delegatedAgentMaxTurns; turn++ {
			reply, err := llm.Chat(ctx, messages)
			if err != nil {
				telemetry.Logger().Error("agent_call_failed", "trace_id", traceID, "agent_id", agentID, "turn", turn+1, "error", err.Error())
				return "", fmt.Errorf("agent.call: %w", err)
			}

			calls, thinking, hasCalls := parseAgentToolCalls(reply)
			if !hasCalls {
				final := strings.TrimSpace(reply)
				telemetry.Logger().Info("agent_call_done", "trace_id", traceID, "agent_id", agentID, "reply_chars", len(final), "turn", turn+1)
				return final, nil
			}

			toolResults := make([]string, 0, len(calls))
			for _, call := range calls {
				if strings.EqualFold(strings.TrimSpace(call.Name), "agent.call") {
					toolResults = append(toolResults, "TOOL_RESULT name=agent.call\ntool execution error: recursive delegation is not allowed")
					continue
				}

				telemetry.Logger().Info("agent_tool_call_requested", "trace_id", traceID, "agent_id", agentID, "tool", call.Name, "input_chars", len(call.Input), "turn", turn+1)
				result, err := toolExec.ExecuteTool(ctx, call.Name, call.Input)
				if err != nil {
					telemetry.Logger().Error("agent_tool_call_failed", "trace_id", traceID, "agent_id", agentID, "tool", call.Name, "turn", turn+1, "error", err.Error())
					result = "tool execution error: " + err.Error()
				}
				telemetry.Logger().Info("agent_tool_call_done", "trace_id", traceID, "agent_id", agentID, "tool", call.Name, "turn", turn+1, "result_chars", len(result))
				toolResults = append(toolResults, fmt.Sprintf("TOOL_RESULT name=%s\n%s", call.Name, result))
			}

			toolResultBlock := strings.Join(toolResults, "\n\n")
			if strings.TrimSpace(thinking) == "" {
				thinking = "Using tool results to continue."
			}
			messages = append(messages,
				domain.Message{Role: domain.RoleAssistant, Content: strings.TrimSpace(reply)},
				domain.Message{Role: domain.RoleUser, Content: strings.TrimSpace(thinking) + "\n\n" + toolResultBlock},
			)
		}

		telemetry.Logger().Error("agent_call_max_turns_reached", "trace_id", traceID, "agent_id", agentID, "max_turns", delegatedAgentMaxTurns)
		return "", fmt.Errorf("agent.call: max tool iterations reached")
	}
}

func buildDelegatedAgentMessages(ctx context.Context, agent *domain.GenericAgent, prompt string, toolExec ports.ToolExecutor) ([]domain.Message, error) {
	messages := agent.BuildMessages(prompt)
	toolsList, err := toolExec.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	lines := []string{
		"CRITICAL: When a tool is needed, you MUST reply with EXACTLY this format on its own line:",
		"TOOL_CALL {\"name\":\"tool.name\",\"input\":\"text input\"}",
		"Do NOT use any other format like <tool_call>, <function=...>, or markdown code blocks.",
		"Do NOT use agent.call — it does not exist for you.",
		"If no tool is needed, answer normally without TOOL_CALL.",
		"Available tools:",
	}
	for _, tool := range toolsList {
		name := strings.TrimSpace(tool.Name)
		if name == "" || strings.EqualFold(name, "agent.call") {
			continue
		}
		desc := strings.TrimSpace(tool.Description)
		if desc == "" {
			desc = "no description"
		}
		lines = append(lines, "- "+name+": "+desc)
	}

	instruction := strings.Join(lines, "\n")
	if len(messages) > 0 && messages[0].Role == domain.RoleSystem {
		messages[0].Content = strings.TrimSpace(messages[0].Content + "\n\n" + instruction)
		return messages, nil
	}

	withSystem := make([]domain.Message, 0, len(messages)+1)
	withSystem = append(withSystem, domain.Message{Role: domain.RoleSystem, Content: instruction})
	withSystem = append(withSystem, messages...)
	return withSystem, nil
}

type toolCall struct {
	Name  string
	Input string
}

type toolCallRaw struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func parseAgentToolCalls(reply string) ([]toolCall, string, bool) {
	// Try standard TOOL_CALL format first.
	if idx := strings.Index(reply, "TOOL_CALL"); idx >= 0 {
		thinking := strings.TrimSpace(reply[:idx])
		payload := strings.TrimSpace(reply[idx+len("TOOL_CALL"):])
		if payload != "" {
			if calls, ok := parseJSONToolCalls(payload); ok {
				return calls, thinking, true
			}
		}
	}

	// Try XML <tool_call> format that some models emit.
	if calls, thinking, ok := parseXMLToolCalls(reply); ok {
		return calls, thinking, true
	}

	return nil, "", false
}

func parseJSONToolCalls(payload string) ([]toolCall, bool) {
	var many []toolCallRaw
	if err := json.Unmarshal([]byte(payload), &many); err == nil {
		calls := make([]toolCall, 0, len(many))
		for _, call := range many {
			name := strings.TrimSpace(call.Name)
			if name == "" {
				continue
			}
			input, ok := normalizeToolInput(call.Input)
			if !ok {
				continue
			}
			calls = append(calls, toolCall{Name: name, Input: input})
		}
		if len(calls) == 0 {
			return nil, false
		}
		return calls, true
	}

	var one toolCallRaw
	if err := json.Unmarshal([]byte(payload), &one); err != nil {
		return nil, false
	}
	name := strings.TrimSpace(one.Name)
	if name == "" {
		return nil, false
	}
	input, ok := normalizeToolInput(one.Input)
	if !ok {
		return nil, false
	}
	return []toolCall{{Name: name, Input: input}}, true
}

// parseXMLToolCalls handles models that emit:
// <tool_call>
// <function=native.list_dirs>
// <parameter=path>docs</parameter>
// </function>
// </tool_call>
func parseXMLToolCalls(reply string) ([]toolCall, string, bool) {
	const openTag = "<tool_call>"
	const closeTag = "</tool_call>"

	idx := strings.Index(reply, openTag)
	if idx < 0 {
		return nil, "", false
	}

	thinking := strings.TrimSpace(reply[:idx])
	calls := make([]toolCall, 0, 2)

	remaining := reply[idx:]
	for {
		start := strings.Index(remaining, openTag)
		if start < 0 {
			break
		}
		end := strings.Index(remaining[start:], closeTag)
		if end < 0 {
			// Handle unclosed tag: take rest of string
			end = len(remaining) - start
		} else {
			end += len(closeTag)
		}
		block := remaining[start : start+end]
		remaining = remaining[start+end:]

		name, input := parseXMLFunctionCall(block)
		if name != "" {
			calls = append(calls, toolCall{Name: name, Input: input})
		}
	}

	if len(calls) == 0 {
		return nil, thinking, false
	}
	return calls, thinking, true
}

func parseXMLFunctionCall(block string) (string, string) {
	// Extract function name from <function=name>
	const funcPrefix = "<function="
	fStart := strings.Index(block, funcPrefix)
	if fStart < 0 {
		return "", ""
	}
	fStart += len(funcPrefix)
	fEnd := strings.Index(block[fStart:], ">")
	if fEnd < 0 {
		return "", ""
	}
	name := strings.TrimSpace(block[fStart : fStart+fEnd])

	// Extract parameter values (join all <parameter=key>value</parameter>)
	params := make(map[string]string)
	search := block[fStart+fEnd:]
	for {
		const paramPrefix = "<parameter="
		pStart := strings.Index(search, paramPrefix)
		if pStart < 0 {
			break
		}
		pStart += len(paramPrefix)
		keyEnd := strings.Index(search[pStart:], ">")
		if keyEnd < 0 {
			break
		}
		key := strings.TrimSpace(search[pStart : pStart+keyEnd])
		valStart := pStart + keyEnd + 1
		valEnd := strings.Index(search[valStart:], "</parameter>")
		var val string
		if valEnd < 0 {
			val = strings.TrimSpace(search[valStart:])
		} else {
			val = strings.TrimSpace(search[valStart : valStart+valEnd])
		}
		params[key] = val
		if valEnd < 0 {
			break
		}
		search = search[valStart+valEnd+len("</parameter>"):]
	}

	// Build input: if single parameter, use value directly; if multiple, JSON-encode
	var input string
	switch len(params) {
	case 0:
		input = ""
	case 1:
		for _, v := range params {
			input = v
		}
	default:
		b, _ := json.Marshal(params)
		input = string(b)
	}

	return name, input
}

func normalizeToolInput(raw json.RawMessage) (string, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", true
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, true
	}

	var asAny any
	if err := json.Unmarshal(raw, &asAny); err != nil {
		return "", false
	}

	b, err := json.Marshal(asAny)
	if err != nil {
		return "", false
	}
	return string(b), true
}

func parseListDirsInput(input string) (string, int, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ".", 0, nil
	}

	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Path  string `json:"path"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return "", 0, fmt.Errorf("invalid JSON input: %w", err)
		}
		path := strings.TrimSpace(payload.Path)
		if path == "" {
			path = "."
		}
		if payload.Limit < 0 {
			payload.Limit = 0
		}
		return path, payload.Limit, nil
	}

	if n, err := strconv.Atoi(trimmed); err == nil {
		if n < 0 {
			n = 0
		}
		return ".", n, nil
	}

	return trimmed, 0, nil
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func inferContextWindow(model string) int {
	name := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(name, "gpt-4.1"):
		return 1_000_000
	case strings.Contains(name, "gpt-4o"), strings.Contains(name, "o4"):
		return 128_000
	default:
		return 128_000
	}
}

func parseAgentCallInput(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("agent.call: input cannot be empty")
	}

	var payload agentToolInput
	if strings.HasPrefix(raw, "{") {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return "", "", fmt.Errorf("agent.call: invalid JSON input: %w", err)
		}
	} else {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("agent.call: input must be JSON or '<agent_id>: <prompt>'")
		}
		payload.AgentID = strings.TrimSpace(parts[0])
		payload.Prompt = strings.TrimSpace(parts[1])
	}

	agentID := strings.TrimSpace(payload.AgentID)
	prompt := strings.TrimSpace(payload.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(payload.Task)
	}
	if prompt == "" {
		prompt = strings.TrimSpace(payload.Message)
	}

	if agentID == "" {
		return "", "", fmt.Errorf("agent.call: agent_id is required")
	}
	if prompt == "" {
		return "", "", fmt.Errorf("agent.call: prompt is required")
	}
	return agentID, prompt, nil
}
