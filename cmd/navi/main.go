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
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	navcmd "navi/cmd/navi/cmd"
	llmadapter "navi/internal/adapters/llm/openai"
	"navi/internal/adapters/mcp/inprocess"
	"navi/internal/adapters/tools"
	"navi/internal/config"
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

	toolRegistry := tools.NewRegistry()
	_ = toolRegistry.Register("native.now", "Current UTC time in RFC3339", func(_ context.Context, _ string) (string, error) {
		return time.Now().UTC().Format(time.RFC3339), nil
	})
	_ = toolRegistry.Register("native.echo", "Echo input text", func(_ context.Context, input string) (string, error) {
		return strings.TrimSpace(input), nil
	})

	mcpClient := inprocess.New()
	_ = mcpClient.Register("echo", func(_ context.Context, input string) (string, error) {
		return "mcp echo: " + strings.TrimSpace(input), nil
	})
	_ = toolRegistry.Register("mcp.echo", "Call MCP echo tool", func(ctx context.Context, input string) (string, error) {
		return mcpClient.CallTool(ctx, "echo", input)
	})

	orchestratorService := orchestratorsvc.New(adapter, toolRegistry)

	deps := navcmd.Dependencies{
		Chat:         chatService,
		Tasks:        nil, // serve command lazily wires SQLite-backed task service
		Agents:       nil, // serve command lazily wires SQLite-backed agent service
		Orchestrator: orchestratorService,
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
