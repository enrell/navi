package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"navi/internal/config"
)

// ── DefaultConfig ─────────────────────────────────────────────────────────────

func TestDefaultConfig_Provider(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.DefaultLLM.Provider != config.ProviderNVIDIA {
		t.Errorf("default provider = %q, want %q", cfg.DefaultLLM.Provider, config.ProviderNVIDIA)
	}
}

func TestDefaultConfig_APIKeyEnv(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.DefaultLLM.APIKeyEnv == "" {
		t.Error("default APIKeyEnv should not be empty")
	}
}

// ── Dir / Path ────────────────────────────────────────────────────────────────

func TestDir_ContainsNavi(t *testing.T) {
	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if !strings.HasSuffix(dir, "navi") && !strings.HasSuffix(dir, "navi"+string(os.PathSeparator)) {
		t.Errorf("Dir() = %q, expected to end with 'navi'", dir)
	}
}

func TestPath_EndsWithConfigToml(t *testing.T) {
	p, err := config.Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if filepath.Base(p) != "config.toml" {
		t.Errorf("Path() = %q, expected to end with config.toml", p)
	}
}

func TestPath_UnderDir(t *testing.T) {
	dir, _ := config.Dir()
	p, _ := config.Path()
	if filepath.Dir(p) != dir {
		t.Errorf("Path() %q is not under Dir() %q", p, dir)
	}
}

// ── LoadFrom ──────────────────────────────────────────────────────────────────

func TestLoadFrom_MissingFile_ReturnsDefault(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	// Should fall back to default.
	if cfg.DefaultLLM.Provider != config.ProviderNVIDIA {
		t.Errorf("provider = %q, want nvidia", cfg.DefaultLLM.Provider)
	}
}

func TestLoadFrom_ValidFile_AllProviders(t *testing.T) {
	providers := []struct {
		name   string
		apiEnv string
	}{
		{config.ProviderNVIDIA, "NVIDIA_API_KEY"},
		{config.ProviderOpenAI, "OPENAI_API_KEY"},
		{config.ProviderGroq, "GROQ_API_KEY"},
		{config.ProviderOpenRouter, "OPENROUTER_API_KEY"},
		{config.ProviderOllama, ""},
	}

	for _, tc := range providers {
		t.Run(tc.name, func(t *testing.T) {
			toml := "[default_llm]\nprovider = \"" + tc.name + "\"\napi_key_env = \"" + tc.apiEnv + "\"\n"
			path := writeTemp(t, toml)

			cfg, err := config.LoadFrom(path)
			if err != nil {
				t.Fatalf("LoadFrom error: %v", err)
			}
			if cfg.DefaultLLM.Provider != tc.name {
				t.Errorf("provider = %q, want %q", cfg.DefaultLLM.Provider, tc.name)
			}
		})
	}
}

func TestLoadFrom_ModelOverride(t *testing.T) {
	path := writeTemp(t, "[default_llm]\nprovider=\"nvidia\"\nmodel=\"custom-model\"\napi_key_env=\"NVIDIA_API_KEY\"\n")
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultLLM.Model != "custom-model" {
		t.Errorf("model = %q, want custom-model", cfg.DefaultLLM.Model)
	}
}

func TestLoadFrom_BaseURLOverride(t *testing.T) {
	path := writeTemp(t, "[default_llm]\nprovider=\"nvidia\"\napi_key_env=\"X\"\nbase_url=\"http://proxy:8080/v1\"\n")
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultLLM.BaseURL != "http://proxy:8080/v1" {
		t.Errorf("base_url = %q, want http://proxy:8080/v1", cfg.DefaultLLM.BaseURL)
	}
}

func TestLoadFrom_UnknownProvider_ReturnsError(t *testing.T) {
	path := writeTemp(t, "[default_llm]\nprovider=\"unknown-provider\"\n")
	_, err := config.LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("error %q should mention 'unknown provider'", err.Error())
	}
}

func TestLoadFrom_InvalidTOML_ReturnsError(t *testing.T) {
	path := writeTemp(t, "this is not valid toml ][[[")
	_, err := config.LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error %q should mention 'parse'", err.Error())
	}
}

func TestLoadFrom_UnreadableFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := os.WriteFile(path, []byte("[default_llm]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Skip("cannot set file permissions on this OS")
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	// Verify the permission actually blocks reading; on Windows chmod is a no-op
	// for the owning user, so we skip rather than produce a false negative.
	if _, readErr := os.ReadFile(path); readErr == nil {
		t.Skip("OS does not enforce permission 0o000 for the owner (Windows ACL); skipping")
	}

	_, err := config.LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("error %q should mention 'read'", err.Error())
	}
}

// ── ResolveAPIKey ─────────────────────────────────────────────────────────────

func TestResolveAPIKey_Found(t *testing.T) {
	t.Setenv("TEST_MY_KEY", "secret-123")
	cfg := config.Config{
		DefaultLLM: config.LLMConfig{APIKeyEnv: "TEST_MY_KEY"},
	}
	key, err := cfg.ResolveAPIKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "secret-123" {
		t.Errorf("key = %q, want secret-123", key)
	}
}

func TestResolveAPIKey_Missing(t *testing.T) {
	t.Setenv("TEST_MISSING_KEY", "")
	cfg := config.Config{
		DefaultLLM: config.LLMConfig{APIKeyEnv: "TEST_MISSING_KEY"},
	}
	_, err := cfg.ResolveAPIKey()
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "TEST_MISSING_KEY") {
		t.Errorf("error %q should mention 'TEST_MISSING_KEY'", err.Error())
	}
}

func TestResolveAPIKey_NoEnvRequired(t *testing.T) {
	// Ollama and local providers set APIKeyEnv = ""
	cfg := config.Config{
		DefaultLLM: config.LLMConfig{APIKeyEnv: ""},
	}
	key, err := cfg.ResolveAPIKey()
	if err != nil {
		t.Fatalf("unexpected error for empty APIKeyEnv: %v", err)
	}
	if key != "" {
		t.Errorf("expected empty key, got %q", key)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}
