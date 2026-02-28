package domain

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestAgent(t *testing.T, dir string, configTOML string, agentMD string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(configTOML), 0644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(agentMD), 0644); err != nil {
		t.Fatalf("write AGENT.md: %v", err)
	}
}

func TestLoadAgentConfig_ValidFull(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "coder")
	writeTestAgent(t, agentDir, `
id = "coder"
type = "generic"
description = "Writes code"
prompt = "AGENT.md"
capabilities = ["filesystem:workspace:rw", "exec:bash,go"]
isolation = "docker"
timeout = "10m"
max_concurrent = 3

[llm]
provider = "openai"
model = "gpt-4o"
api_key = "sk-test"
base_url = "https://custom.api/v1"
temperature = 0.5
max_tokens = 2048

[isolation_config]
image = "node:20"
`, "You are a coder agent.")

	cfg, err := LoadAgentConfig(agentDir)
	if err != nil {
		t.Fatalf("LoadAgentConfig error: %v", err)
	}

	if cfg.ID != "coder" {
		t.Errorf("ID = %q, want coder", cfg.ID)
	}
	if cfg.Name != "coder" {
		t.Errorf("Name = %q, want coder", cfg.Name)
	}
	if cfg.Description != "Writes code" {
		t.Errorf("Description = %q", cfg.Description)
	}
	if cfg.Type != "generic" {
		t.Errorf("Type = %q, want generic", cfg.Type)
	}
	if cfg.SystemPrompt != "You are a coder agent." {
		t.Errorf("SystemPrompt = %q", cfg.SystemPrompt)
	}
	if cfg.LLMProvider != "openai" {
		t.Errorf("LLMProvider = %q", cfg.LLMProvider)
	}
	if cfg.LLMModel != "gpt-4o" {
		t.Errorf("LLMModel = %q", cfg.LLMModel)
	}
	if cfg.LLMAPIKey != "sk-test" {
		t.Errorf("LLMAPIKey = %q", cfg.LLMAPIKey)
	}
	if cfg.LLMBaseURL != "https://custom.api/v1" {
		t.Errorf("LLMBaseURL = %q", cfg.LLMBaseURL)
	}
	if cfg.LLMTemperature != 0.5 {
		t.Errorf("LLMTemperature = %f", cfg.LLMTemperature)
	}
	if cfg.LLMMaxTokens != 2048 {
		t.Errorf("LLMMaxTokens = %d", cfg.LLMMaxTokens)
	}
	if len(cfg.Capabilities) != 2 {
		t.Fatalf("Capabilities len = %d, want 2", len(cfg.Capabilities))
	}
	if cfg.Capabilities[0].Raw() != "filesystem:workspace:rw" {
		t.Errorf("Capabilities[0] = %q", cfg.Capabilities[0].Raw())
	}
	if cfg.IsolationType != "docker" {
		t.Errorf("IsolationType = %q", cfg.IsolationType)
	}
	if cfg.IsolationConfig["image"] != "node:20" {
		t.Errorf("IsolationConfig[image] = %q", cfg.IsolationConfig["image"])
	}
	if cfg.Timeout.Minutes() != 10 {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
	if cfg.MaxConcurrent != 3 {
		t.Errorf("MaxConcurrent = %d", cfg.MaxConcurrent)
	}
}

func TestLoadAgentConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "minimal")
	writeTestAgent(t, agentDir, `
id = "minimal"
prompt = "AGENT.md"

[llm]
provider = "openai"
model = "gpt-4o-mini"
`, "Minimal agent.")

	cfg, err := LoadAgentConfig(agentDir)
	if err != nil {
		t.Fatalf("LoadAgentConfig error: %v", err)
	}

	if cfg.Type != "generic" {
		t.Errorf("default Type = %q, want generic", cfg.Type)
	}
	if cfg.IsolationType != "native" {
		t.Errorf("default IsolationType = %q, want native", cfg.IsolationType)
	}
	if cfg.LLMTemperature != 0.7 {
		t.Errorf("default LLMTemperature = %f, want 0.7", cfg.LLMTemperature)
	}
	if cfg.LLMMaxTokens != 4096 {
		t.Errorf("default LLMMaxTokens = %d, want 4096", cfg.LLMMaxTokens)
	}
	if cfg.Timeout.Minutes() != 30 {
		t.Errorf("default Timeout = %v, want 30m", cfg.Timeout)
	}
	if cfg.MaxConcurrent != 5 {
		t.Errorf("default MaxConcurrent = %d, want 5", cfg.MaxConcurrent)
	}
}

func TestLoadAgentConfig_MissingID(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "noid")
	writeTestAgent(t, agentDir, `
prompt = "AGENT.md"
[llm]
provider = "openai"
`, "No id.")

	_, err := LoadAgentConfig(agentDir)
	if err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestLoadAgentConfig_MissingProvider(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "noprov")
	writeTestAgent(t, agentDir, `
id = "noprov"
prompt = "AGENT.md"
[llm]
model = "gpt-4o"
`, "No provider.")

	_, err := LoadAgentConfig(agentDir)
	if err == nil {
		t.Fatal("expected error for missing llm.provider")
	}
}

func TestLoadAgentConfig_MissingPromptField(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "noprompt")
	writeTestAgent(t, agentDir, `
id = "noprompt"
[llm]
provider = "openai"
`, "Orphan prompt.")

	_, err := LoadAgentConfig(agentDir)
	if err == nil {
		t.Fatal("expected error for missing prompt field")
	}
}

func TestLoadAgentConfig_MissingPromptFile(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "nofile")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "config.toml"), []byte(`
id = "nofile"
prompt = "AGENT.md"
[llm]
provider = "openai"
`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadAgentConfig(agentDir)
	if err == nil {
		t.Fatal("expected error for missing AGENT.md file")
	}
}

func TestLoadAgentConfig_BadTOML(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "bad")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "config.toml"), []byte(`{{{not toml`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadAgentConfig(agentDir)
	if err == nil {
		t.Fatal("expected error for bad TOML")
	}
}

func TestLoadAgentConfig_MissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadAgentConfig(dir)
	if err == nil {
		t.Fatal("expected error for missing config.toml")
	}
}

func TestLoadAgentConfig_InvalidCapability(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "badcap")
	writeTestAgent(t, agentDir, `
id = "badcap"
prompt = "AGENT.md"
capabilities = [""]

[llm]
provider = "openai"
`, "Bad cap.")

	_, err := LoadAgentConfig(agentDir)
	if err == nil {
		t.Fatal("expected error for invalid capability")
	}
}

func TestLoadAgentConfig_InvalidTimeout(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "badtime")
	writeTestAgent(t, agentDir, `
id = "badtime"
prompt = "AGENT.md"
timeout = "not-a-duration"

[llm]
provider = "openai"
`, "Bad timeout.")

	_, err := LoadAgentConfig(agentDir)
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}
