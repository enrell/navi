package repl

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"navi/internal/adapters/llm/openai"
	"navi/internal/adapters/storage/sqlite"
	"navi/internal/core/domain"
)

type Repl struct {
	agent *domain.GenericAgent
	db    *sqlite.SQLiteRepository
	cfg   *domain.GlobalConfig
}

func NewRepl(dbPath, apiKey, model, baseURL string) (*Repl, error) {
	// Load global config from ~/.config/navi/config.toml
	cfg, err := domain.LoadGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Determine effective values: CLI args > config > error
	provider := cfg.GetDefaultProvider()
	effectiveModel := model
	effectiveBaseURL := baseURL
	effectiveAPIKey := apiKey

	// If no model provided via CLI, check config
	if effectiveModel == "" {
		effectiveModel = cfg.GetDefaultModel()
	}

	// Validate: user must have a provider configured
	if !cfg.HasProvider() {
		return nil, fmt.Errorf("no LLM provider configured. Please add your provider configuration to ~/.config/navi/config.toml\n\n" +
			"Example config.toml:\n" +
			"[default_llm]\n" +
			"provider = \"openai\"\n" +
			"model = \"gpt-4o-mini\"\n")
	}

	// Validate: user must have an API key
	hasEnvKey := cfg.HasAPIKey()
	if effectiveAPIKey == "" && !hasEnvKey {
		return nil, fmt.Errorf("no API key found. Please either:\n" +
			"  1. Set OPENAI_API_KEY (or NVIDIA_API_KEY, OPENROUTER_API_KEY, LLM_API_KEY) environment variable\n" +
			"  2. Add api_key to your config.toml")
	}

	// Use base URL from config if not provided via CLI
	if effectiveBaseURL == "" {
		switch strings.ToLower(provider) {
		case "nvidia":
			effectiveBaseURL = "https://integrate.api.nvidia.com/v1"
		case "openrouter":
			effectiveBaseURL = "https://openrouter.ai/api/v1"
		}
	}

	// Validate: model is required
	if effectiveModel == "" {
		return nil, fmt.Errorf("no default model configured. Please add model to ~/.config/navi/config.toml:\n\n" +
			"[default_llm]\n" +
			"provider = \"openai\"\n" +
			"model = \"gpt-4o-mini\"")
	}

	db, err := sqlite.NewSQLiteRepository(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init db: %w", err)
	}

	// Get temperature and max tokens from config or use defaults
	temperature := cfg.DefaultLLM.Temperature
	if temperature == 0 {
		temperature = 0.7
	}
	maxTokens := cfg.DefaultLLM.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	llm, err := openai.New(effectiveAPIKey, effectiveModel, effectiveBaseURL, temperature, maxTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM adapter: %w", err)
	}

	// Use system prompt from config or default
	systemPrompt := cfg.Repl.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are a helpful assistant living inside a CLI REPL. You must be concise and precise."
	}

	agentCfg := domain.AgentConfig{
		ID:           "repl-agent",
		Name:         "REPL CLI Agent",
		SystemPrompt: systemPrompt,
	}
	agent := domain.NewGenericAgent(agentCfg, llm, nil, nil)

	return &Repl{
		agent: agent,
		db:    db,
		cfg:   cfg,
	}, nil
}

func (r *Repl) Run(ctx context.Context) error {
	sessionID := uuid.New().String()
	_, err := r.db.CreateSession(ctx, sessionID, "CLI Session "+time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return err
	}

	fmt.Printf("NAVI CLI REPL (Session: %s)\n", sessionID)
	fmt.Println("Type 'exit' to quit")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("Using: %s / %s\n", r.cfg.GetDefaultProvider(), r.cfg.GetDefaultModel())

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if strings.ToLower(input) == "exit" {
			break
		}

		userMsg := domain.ChatMessage{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			AgentID:   r.agent.ID(),
			UserID:    "user",
			Role:      domain.ChatRoleUser,
			Content:   input,
			Timestamp: time.Time{},
		}
		userMsg.Timestamp = time.Now()
		if err := r.db.AddMessage(ctx, userMsg); err != nil {
			fmt.Printf("[Error saving user msg: %v]\n", err)
		}

		task := domain.Task{
			ID:      uuid.New().String(),
			Prompt:  input,
			AgentID: r.agent.ID(),
		}

		start := time.Now()
		result, err := r.agent.Execute(ctx, task)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("[Agent Error: %v]\n", err)
			r.logEvent(ctx, task.ID, sessionID, input, result.Output, err.Error())
			continue
		}

		aiMsg := domain.ChatMessage{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			AgentID:   r.agent.ID(),
			UserID:    "agent",
			Role:      domain.ChatRoleAssistant,
			Content:   result.Output,
			Timestamp: time.Now(),
		}
		if err := r.db.AddMessage(ctx, aiMsg); err != nil {
			fmt.Printf("[Error saving AI msg: %v]\n", err)
		}

		r.logEvent(ctx, task.ID, sessionID, input, result.Output, "")

		fmt.Printf("\nAgent (%s): %s\n", duration.Round(time.Millisecond), result.Output)
	}

	return nil
}

func (r *Repl) logEvent(ctx context.Context, taskID, sessionID, prompt, result, errorMsg string) {
	evt := domain.Event{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		AgentID:   r.agent.ID(),
		UserID:    "repl",
		Type:      domain.EventTaskCompleted,
		Result:    result,
		Error:     errorMsg,
		Metadata: map[string]any{
			"task_id":    taskID,
			"session_id": sessionID,
			"prompt":     prompt,
		},
	}
	_ = r.db.Record(ctx, evt)
}

func (r *Repl) Close() error {
	return r.db.Close()
}

// ensureConfigDir ensures the config directory exists
func ensureConfigDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(home, ".config", "navi")
	return os.MkdirAll(configDir, 0755)
}
