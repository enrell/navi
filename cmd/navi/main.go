// Navi — secure AI orchestrator.
//
// This file is the composition root: it reads environment/config, wires all
// adapters and services together, then hands off to cobra.
//
// Keep it thin. Business logic belongs in internal/core/services/.
// Adapter choices belong in internal/adapters/.
package main

import (
	"fmt"
	"io"
	"os"

	navcmd "navi/cmd/navi/cmd"
	llmadapter "navi/internal/adapters/llm/openai"
	"navi/internal/adapters/persistence/memory"
	"navi/internal/config"
	agentsvc "navi/internal/core/services/agent"
	"navi/internal/core/services/chat"
	tasksvc "navi/internal/core/services/task"
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
	llmCfg, err := buildLLMConfig()
	if err != nil {
		return err
	}

	// Wire: pkg HTTP client → adapter (satisfies LLMPort) → chat service
	adapter := llmadapter.New(llmCfg)
	chatService := chat.New(adapter)

	// Wire: in-memory repos → task / agent services
	// (SQLite persistence is a future iteration)
	taskRepo := memory.NewTaskRepository()
	agentRepo := memory.NewAgentRepository(nil) // no agents registered yet
	taskService := tasksvc.New(taskRepo, chatService)
	agentService := agentsvc.New(agentRepo)

	deps := navcmd.Dependencies{
		Chat:   chatService,
		Tasks:  taskService,
		Agents: agentService,
	}

	root := navcmd.NewRootCommand(deps, out)
	root.SetArgs(args)
	return root.Execute()
}

// configPath is a seam for tests that need to simulate config.Path() failures.
var configPath = config.Path

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
		model := llm.Model
		if model == "" {
			model = "llama3.1:8b"
		}
		preset = llmpkg.Ollama(model)
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
