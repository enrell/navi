// Package config loads and validates Navi's configuration file.
//
// Platform paths (via os.UserConfigDir):
//
//	Linux   : $XDG_CONFIG_HOME/navi/config.toml  (~/.config/navi/config.toml)
//	macOS   : ~/Library/Application Support/navi/config.toml
//	Windows : %AppData%\navi\config.toml
//
// If the config file does not exist, DefaultConfig() is returned so the binary
// works out-of-the-box with only an environment variable set.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Provider names — the allowed values for LLMConfig.Provider.
const (
	ProviderNVIDIA     = "nvidia"
	ProviderOpenAI     = "openai"
	ProviderGroq       = "groq"
	ProviderOpenRouter = "openrouter"
	ProviderOllama     = "ollama"
)

// LLMConfig holds the settings for the language model backend.
type LLMConfig struct {
	// Provider selects the backend preset.
	// Allowed: nvidia | openai | groq | openrouter | ollama
	Provider string `toml:"provider"`

	// Model overrides the provider's default model name.
	// Leave empty to use the provider preset default.
	Model string `toml:"model"`

	// APIKeyEnv is the name of the environment variable that holds the API key.
	// Example: "NVIDIA_API_KEY", "OPENAI_API_KEY".
	// Not used for Ollama (local, no auth).
	APIKeyEnv string `toml:"api_key_env"`

	// BaseURL overrides the provider's default endpoint.
	// Useful for proxies, local deployments, or testing.
	BaseURL string `toml:"base_url"`
}

// Config is the top-level configuration structure.
type Config struct {
	DefaultLLM LLMConfig `toml:"default_llm"`
}

// DefaultConfig returns a working configuration that requires only
// NVIDIA_API_KEY to be set in the environment — no config file needed.
func DefaultConfig() Config {
	return Config{
		DefaultLLM: LLMConfig{
			Provider:  ProviderNVIDIA,
			APIKeyEnv: "NVIDIA_API_KEY",
		},
	}
}

// userConfigDir is a seam for tests to override os.UserConfigDir.
var userConfigDir = os.UserConfigDir

// Dir returns the platform-appropriate directory where navi stores its config.
// It never creates the directory — just returns the expected path.
func Dir() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: cannot determine user config directory: %w", err)
	}
	return filepath.Join(base, "navi"), nil
}

// Path returns the full path to the config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads the config from the platform-appropriate path.
// If the file does not exist, DefaultConfig is returned with no error.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	return LoadFrom(path)
}

// LoadFrom reads and parses the config from the given path.
// If the file does not exist, DefaultConfig is returned with no error.
// Use this in tests to point at a temp file.
func LoadFrom(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("config: read %q: %w", path, err)
	}

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse %q: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}

	return cfg, nil
}

// ResolveAPIKey looks up the API key from the environment variable named in
// cfg.DefaultLLM.APIKeyEnv. Returns an error if the variable is unset or empty.
func (c Config) ResolveAPIKey() (string, error) {
	envName := c.DefaultLLM.APIKeyEnv
	if envName == "" {
		// Ollama and other local providers don't need a key.
		return "", nil
	}
	key := os.Getenv(envName)
	if key == "" {
		return "", fmt.Errorf("%s environment variable is not set\n"+
			"  Set it or add it to ~/.config/navi/config.toml via api_key_env",
			envName)
	}
	return key, nil
}

// validate checks that the config contains only supported values.
// It also normalises Provider to lowercase so callers can rely on the constant
// values (config.ProviderNVIDIA etc.) matching exactly.
func (c *Config) validate() error {
	valid := map[string]bool{
		ProviderNVIDIA:     true,
		ProviderOpenAI:     true,
		ProviderGroq:       true,
		ProviderOpenRouter: true,
		ProviderOllama:     true,
	}
	p := strings.ToLower(c.DefaultLLM.Provider)
	if p != "" && !valid[p] {
		return fmt.Errorf("unknown provider %q — must be one of: nvidia, openai, groq, openrouter, ollama", c.DefaultLLM.Provider)
	}
	// Normalise to lowercase so consumers can rely on the constant values.
	c.DefaultLLM.Provider = p
	return nil
}
