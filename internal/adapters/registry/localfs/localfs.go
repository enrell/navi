// Package localfs implements ports.AgentConfigRegistry by scanning
// ~/.config/navi/agents/*/ directories on the local filesystem.
package localfs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"navi/internal/core/domain"
)

// LocalFSRegistry reads and writes agent configs from the filesystem.
type LocalFSRegistry struct {
	baseDir string // e.g. ~/.config/navi/agents
}

// New creates a LocalFSRegistry rooted at baseDir.
// If baseDir is empty, it defaults to ~/.config/navi/agents.
func New(baseDir string) (*LocalFSRegistry, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("localfs: resolve home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".config", "navi", "agents")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("localfs: create agents dir: %w", err)
	}
	return &LocalFSRegistry{baseDir: baseDir}, nil
}

// LoadAll scans baseDir for subdirectories containing config.toml and
// returns the parsed AgentConfig for each valid agent directory.
func (r *LocalFSRegistry) LoadAll() ([]domain.AgentConfig, error) {
	entries, err := os.ReadDir(r.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("localfs: read agents dir: %w", err)
	}

	var configs []domain.AgentConfig
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(r.baseDir, entry.Name())
		cfg, err := domain.LoadAgentConfig(dir)
		if err != nil {
			// Warn but skip invalid configs
			fmt.Fprintf(os.Stderr, "[navi] skipping agent %q: %v\n", entry.Name(), err)
			continue
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// Save writes config.toml and AGENT.md for the given AgentConfig.
// It creates the agent directory if it doesn't exist.
func (r *LocalFSRegistry) Save(cfg domain.AgentConfig) error {
	if cfg.ID == "" {
		return fmt.Errorf("localfs: agent config missing ID")
	}
	agentDir := filepath.Join(r.baseDir, cfg.ID)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("localfs: create agent dir: %w", err)
	}

	// Determine prompt filename
	promptFile := cfg.PromptFile
	if promptFile == "" {
		promptFile = "AGENT.md"
	}

	// Serialize capabilities back to strings
	rawCaps := make([]string, len(cfg.Capabilities))
	for i, c := range cfg.Capabilities {
		rawCaps[i] = c.Raw()
	}

	// Build TOML-serializable struct
	type llmSection struct {
		Provider    string  `toml:"provider"`
		Model       string  `toml:"model"`
		APIKey      string  `toml:"api_key,omitempty"`
		BaseURL     string  `toml:"base_url,omitempty"`
		Temperature float64 `toml:"temperature"`
		MaxTokens   int     `toml:"max_tokens"`
	}
	type configFile struct {
		ID              string            `toml:"id"`
		Type            string            `toml:"type"`
		Description     string            `toml:"description,omitempty"`
		Prompt          string            `toml:"prompt"`
		LLM             llmSection        `toml:"llm"`
		Capabilities    []string          `toml:"capabilities"`
		Isolation       string            `toml:"isolation"`
		IsolationConfig map[string]string `toml:"isolation_config,omitempty"`
		Timeout         string            `toml:"timeout,omitempty"`
		MaxConcurrent   int               `toml:"max_concurrent"`
	}

	raw := configFile{
		ID:          cfg.ID,
		Type:        cfg.Type,
		Description: cfg.Description,
		Prompt:      promptFile,
		LLM: llmSection{
			Provider:    cfg.LLMProvider,
			Model:       cfg.LLMModel,
			APIKey:      cfg.LLMAPIKey,
			BaseURL:     cfg.LLMBaseURL,
			Temperature: cfg.LLMTemperature,
			MaxTokens:   cfg.LLMMaxTokens,
		},
		Capabilities:    rawCaps,
		Isolation:       cfg.IsolationType,
		IsolationConfig: cfg.IsolationConfig,
		MaxConcurrent:   cfg.MaxConcurrent,
	}
	if cfg.Timeout > 0 {
		raw.Timeout = cfg.Timeout.String()
	}

	// Write config.toml
	cfgPath := filepath.Join(agentDir, "config.toml")
	f, err := os.Create(cfgPath)
	if err != nil {
		return fmt.Errorf("localfs: create config.toml: %w", err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(raw); err != nil {
		return fmt.Errorf("localfs: encode config.toml: %w", err)
	}

	// Write AGENT.md (system prompt)
	promptPath := filepath.Join(agentDir, promptFile)
	if err := os.WriteFile(promptPath, []byte(cfg.SystemPrompt), 0644); err != nil {
		return fmt.Errorf("localfs: write AGENT.md: %w", err)
	}
	return nil
}

// Delete removes the entire agent directory.
func (r *LocalFSRegistry) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("localfs: empty agent ID")
	}
	agentDir := filepath.Join(r.baseDir, id)
	if err := os.RemoveAll(agentDir); err != nil {
		return fmt.Errorf("localfs: delete agent dir %s: %w", id, err)
	}
	return nil
}
