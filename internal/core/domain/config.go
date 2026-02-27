package domain

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// ─── TOML Schema ─────────────────────────────────────────────────────────────

// agentConfigFile mirrors the on-disk config.toml structure exactly.
// All fields are exported so the TOML decoder can populate them.
type agentConfigFile struct {
	ID          string `toml:"id"`
	Type        string `toml:"type"`
	Description string `toml:"description"`
	Prompt      string `toml:"prompt"`

	LLM struct {
		Provider    string  `toml:"provider"`
		Model       string  `toml:"model"`
		APIKey      string  `toml:"api_key"`
		BaseURL     string  `toml:"base_url"`
		Temperature float64 `toml:"temperature"`
		MaxTokens   int     `toml:"max_tokens"`
	} `toml:"llm"`

	Capabilities    []string          `toml:"capabilities"`
	Isolation       string            `toml:"isolation"`
	IsolationConfig map[string]string `toml:"isolation_config"`
	Timeout         string            `toml:"timeout"`
	MaxConcurrent   int               `toml:"max_concurrent"`
}

// ─── Loader ───────────────────────────────────────────────────────────────────

// LoadAgentConfig reads config.toml and the referenced AGENT.md from the
// given agent directory and returns a fully-populated AgentConfig.
func LoadAgentConfig(dir string) (AgentConfig, error) {
	cfgPath := filepath.Join(dir, "config.toml")

	var raw agentConfigFile
	if _, err := toml.DecodeFile(cfgPath, &raw); err != nil {
		return AgentConfig{}, fmt.Errorf("parse %s: %w", cfgPath, err)
	}

	if raw.ID == "" {
		return AgentConfig{}, fmt.Errorf("%s: field 'id' is required", cfgPath)
	}
	if raw.LLM.Provider == "" {
		return AgentConfig{}, fmt.Errorf("%s: field 'llm.provider' is required", cfgPath)
	}
	if raw.Prompt == "" {
		return AgentConfig{}, fmt.Errorf("%s: field 'prompt' is required", cfgPath)
	}

	// Read system prompt file
	promptPath := filepath.Join(dir, raw.Prompt)
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("read prompt file %s: %w", promptPath, err)
	}

	// Parse capabilities
	caps := make([]Capability, 0, len(raw.Capabilities))
	for _, s := range raw.Capabilities {
		c, err := ParseCapability(s)
		if err != nil {
			return AgentConfig{}, fmt.Errorf("invalid capability %q: %w", s, err)
		}
		caps = append(caps, c)
	}

	// Parse timeout
	var timeout time.Duration
	if raw.Timeout != "" {
		timeout, err = time.ParseDuration(raw.Timeout)
		if err != nil {
			return AgentConfig{}, fmt.Errorf("invalid timeout %q: %w", raw.Timeout, err)
		}
	} else {
		timeout = 30 * time.Minute
	}

	maxConcurrent := raw.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}

	temperature := raw.LLM.Temperature
	if temperature == 0 {
		temperature = 0.7
	}

	maxTokens := raw.LLM.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	agentType := raw.Type
	if agentType == "" {
		agentType = "generic"
	}

	isolation := raw.Isolation
	if isolation == "" {
		isolation = "native"
	}

	cfg := AgentConfig{
		ID:              raw.ID,
		Name:            raw.ID,
		Description:     raw.Description,
		Type:            agentType,
		PromptFile:      raw.Prompt,
		SystemPrompt:    string(promptBytes),
		LLMProvider:     raw.LLM.Provider,
		LLMModel:        raw.LLM.Model,
		LLMAPIKey:       raw.LLM.APIKey,
		LLMBaseURL:      raw.LLM.BaseURL,
		LLMTemperature:  temperature,
		LLMMaxTokens:    maxTokens,
		Capabilities:    caps,
		IsolationType:   isolation,
		IsolationConfig: raw.IsolationConfig,
		Timeout:         timeout,
		MaxConcurrent:   maxConcurrent,
	}

	return cfg, nil
}
