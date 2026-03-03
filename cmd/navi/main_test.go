package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"navi/internal/config"
)

func TestEnsureConfigDir_CreatesDirectory(t *testing.T) {
	orig := configDir
	tmp := filepath.Join(t.TempDir(), "navi-config")
	configDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { configDir = orig })

	if err := ensureConfigDir(); err != nil {
		t.Fatalf("ensureConfigDir error: %v", err)
	}
	if fi, err := os.Stat(tmp); err != nil || !fi.IsDir() {
		t.Fatalf("expected dir %q to exist", tmp)
	}
}

func TestEnsureConfigDir_DirError(t *testing.T) {
	orig := configDir
	configDir = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { configDir = orig })

	if err := ensureConfigDir(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadEnvironment_ReadsDotEnv(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	if err := os.WriteFile(filepath.Join(dir, ".env.development"), []byte("NAVI_DEFAULT_MODEL=dev-model\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv("NAVI_ENV", "development")

	loaded, err := loadEnvironment()
	if err != nil {
		t.Fatalf("loadEnvironment error: %v", err)
	}
	if len(loaded) != 1 || loaded[0] != ".env.development" {
		t.Fatalf("loaded files = %+v, want [.env.development]", loaded)
	}
	if got := os.Getenv("NAVI_DEFAULT_MODEL"); got != "dev-model" {
		t.Errorf("NAVI_DEFAULT_MODEL = %q, want dev-model", got)
	}
}

func TestRun_DevelopmentPrintsLoadedEnvFiles(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	if err := os.WriteFile(filepath.Join(dir, ".env.development"), []byte("NAVI_DEFAULT_PROVIDER=ollama\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv("NAVI_ENV", "development")
	t.Setenv("NAVI_LLM_BASE_URL", "")

	var buf bytes.Buffer
	if err := run([]string{"--help"}, &buf); err != nil {
		t.Fatalf("run --help: %v", err)
	}
	if !strings.Contains(buf.String(), "[dev] loaded env files: .env.development") {
		t.Errorf("output %q should contain loaded env files log", buf.String())
	}
}

// Note: main() itself cannot be unit-tested because it calls os.Exit(1) on
// error — that line is the only legitimately uncoverable statement in the file.

// ── buildLLMConfig ────────────────────────────────────────────────────────────

func TestBuildLLMConfig_PathError(t *testing.T) {
	sentinel := errors.New("no home directory")
	orig := configPath
	configPath = func() (string, error) { return "", sentinel }
	t.Cleanup(func() { configPath = orig })

	_, err := buildLLMConfig()
	if err == nil {
		t.Fatal("expected error from buildLLMConfig when configPath fails")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want sentinel %v", err, sentinel)
	}
}

// ── buildLLMConfigFrom ────────────────────────────────────────────────────────

func TestBuildLLMConfigFrom_NoFile_DefaultsToNVIDIA(t *testing.T) {
	clearNaviEnvOverrides(t)
	t.Setenv("NVIDIA_API_KEY", "mykey")
	t.Setenv("NAVI_LLM_BASE_URL", "")

	cfg, err := buildLLMConfigFrom(noConfigPath(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cfg.BaseURL, "nvidia") {
		t.Errorf("BaseURL %q should contain 'nvidia'", cfg.BaseURL)
	}
	if cfg.APIKey != "mykey" {
		t.Errorf("APIKey = %q, want mykey", cfg.APIKey)
	}
}

func TestBuildLLMConfigFrom_MissingAPIKey(t *testing.T) {
	clearNaviEnvOverrides(t)
	t.Setenv("NVIDIA_API_KEY", "")
	_, err := buildLLMConfigFrom(noConfigPath(t))
	if err == nil {
		t.Fatal("expected error when API key env var is unset")
	}
	if !strings.Contains(err.Error(), "NVIDIA_API_KEY") {
		t.Errorf("error %q should mention NVIDIA_API_KEY", err.Error())
	}
}

func TestBuildLLMConfigFrom_ModelOverride(t *testing.T) {
	clearNaviEnvOverrides(t)
	t.Setenv("NVIDIA_API_KEY", "k")
	path := writeCfg(t, `[default_llm]
provider = "nvidia"
api_key_env = "NVIDIA_API_KEY"
model = "custom-model-xyz"`)

	cfg, err := buildLLMConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "custom-model-xyz" {
		t.Errorf("Model = %q, want custom-model-xyz", cfg.Model)
	}
}

func TestBuildLLMConfigFrom_BaseURLOverride_FromFile(t *testing.T) {
	clearNaviEnvOverrides(t)
	t.Setenv("NVIDIA_API_KEY", "k")
	t.Setenv("NAVI_LLM_BASE_URL", "")
	path := writeCfg(t, `[default_llm]
provider = "nvidia"
api_key_env = "NVIDIA_API_KEY"
base_url = "http://proxy:9999/v1"`)

	cfg, err := buildLLMConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "http://proxy:9999/v1" {
		t.Errorf("BaseURL = %q, want http://proxy:9999/v1", cfg.BaseURL)
	}
}

func TestBuildLLMConfigFrom_BaseURLOverride_FromEnv(t *testing.T) {
	clearNaviEnvOverrides(t)
	t.Setenv("NVIDIA_API_KEY", "k")
	t.Setenv("NAVI_LLM_BASE_URL", "http://localhost:9999")

	cfg, err := buildLLMConfigFrom(noConfigPath(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "http://localhost:9999" {
		t.Errorf("BaseURL = %q, want http://localhost:9999", cfg.BaseURL)
	}
}

func TestBuildLLMConfigFrom_EnvProviderAndModelOverride(t *testing.T) {
	t.Setenv("NAVI_DEFAULT_PROVIDER", "openai")
	t.Setenv("NAVI_DEFAULT_MODEL", "gpt-4.1-mini")
	t.Setenv("NAVI_DEFAULT_API_KEY_ENV", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "k-openai")
	t.Setenv("NAVI_LLM_BASE_URL", "")

	cfg, err := buildLLMConfigFrom(noConfigPath(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cfg.BaseURL, "openai") {
		t.Errorf("BaseURL %q should contain openai", cfg.BaseURL)
	}
	if cfg.Model != "gpt-4.1-mini" {
		t.Errorf("Model = %q, want gpt-4.1-mini", cfg.Model)
	}
	if cfg.APIKey != "k-openai" {
		t.Errorf("APIKey = %q, want k-openai", cfg.APIKey)
	}
}

func TestBuildLLMConfigFrom_NaviAPIKeyOverride(t *testing.T) {
	t.Setenv("NAVI_DEFAULT_PROVIDER", "openai")
	t.Setenv("NAVI_API_KEY", "navi-key")
	t.Setenv("NAVI_LLM_BASE_URL", "")

	cfg, err := buildLLMConfigFrom(noConfigPath(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "navi-key" {
		t.Errorf("APIKey = %q, want navi-key", cfg.APIKey)
	}
}

func TestBuildLLMConfigFrom_InvalidEnvProvider(t *testing.T) {
	t.Setenv("NAVI_DEFAULT_PROVIDER", "not-real")
	_, err := buildLLMConfigFrom(noConfigPath(t))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "NAVI_DEFAULT_PROVIDER") {
		t.Errorf("error %q should mention NAVI_DEFAULT_PROVIDER", err.Error())
	}
}

func TestBuildLLMConfigFrom_InvalidTOML(t *testing.T) {
	clearNaviEnvOverrides(t)
	path := writeCfg(t, "not valid toml ][")
	_, err := buildLLMConfigFrom(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

// ── providerPreset ───────────────────────────────────────────────────────────

func TestProviderPreset_AllProviders(t *testing.T) {
	cases := []struct {
		provider    string
		apiKeyEnv   string
		apiKey      string
		wantURLFrag string
	}{
		{config.ProviderNVIDIA, "X", "k", "nvidia"},
		{"", "X", "k", "nvidia"}, // empty defaults to NVIDIA
		{config.ProviderOpenAI, "X", "k", "openai"},
		{config.ProviderGroq, "X", "k", "groq"},
		{config.ProviderOpenRouter, "X", "k", "openrouter"},
		{config.ProviderOllama, "", "", "localhost"},
	}

	for _, tc := range cases {
		t.Run(tc.provider+"_default", func(t *testing.T) {
			cfg := config.Config{DefaultLLM: config.LLMConfig{
				Provider:  tc.provider,
				APIKeyEnv: tc.apiKeyEnv,
			}}
			got := providerPreset(cfg, tc.apiKey)
			if !strings.Contains(got.BaseURL, tc.wantURLFrag) {
				t.Errorf("BaseURL %q does not contain %q", got.BaseURL, tc.wantURLFrag)
			}
		})
	}
}

func TestProviderPreset_OllamaDefaultModel(t *testing.T) {
	cfg := config.Config{DefaultLLM: config.LLMConfig{Provider: config.ProviderOllama}}
	got := providerPreset(cfg, "")
	if got.Model != "" {
		t.Errorf("Model = %q, want empty when not specified", got.Model)
	}
}

func TestProviderPreset_OllamaCustomModel(t *testing.T) {
	cfg := config.Config{DefaultLLM: config.LLMConfig{
		Provider: config.ProviderOllama,
		Model:    "mistral:latest",
	}}
	got := providerPreset(cfg, "")
	if got.Model != "mistral:latest" {
		t.Errorf("Model = %q, want mistral:latest", got.Model)
	}
}

func TestProviderPreset_UppercaseProvider_NormalisedByValidate(t *testing.T) {
	clearNaviEnvOverrides(t)
	// validate() normalises to lowercase before providerPreset is called;
	// verify that loading a config with uppercase provider works end-to-end.
	t.Setenv("NVIDIA_API_KEY", "k")
	path := writeCfg(t, `[default_llm]
provider = "NVIDIA"
api_key_env = "NVIDIA_API_KEY"`)

	cfg, err := buildLLMConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cfg.BaseURL, "nvidia") {
		t.Errorf("BaseURL %q should contain 'nvidia'", cfg.BaseURL)
	}
}

// ── run() ────────────────────────────────────────────────────────────────────

func TestRun_MissingAPIKey(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "")
	t.Setenv("NAVI_LLM_BASE_URL", "")
	err := run([]string{"chat", "hello"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when API key is unset")
	}
	if !strings.Contains(err.Error(), "NVIDIA_API_KEY") {
		t.Errorf("error %q should mention NVIDIA_API_KEY", err.Error())
	}
}

func TestRun_Help(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "k")
	t.Setenv("NAVI_LLM_BASE_URL", "")
	var buf bytes.Buffer
	if err := run([]string{"--help"}, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "navi") {
		t.Errorf("help output %q should mention 'navi'", buf.String())
	}
}

func TestRun_ChatHappyPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "PONG"}},
			},
		})
	}))
	defer ts.Close()

	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NAVI_LLM_BASE_URL", ts.URL)

	var buf bytes.Buffer
	if err := run([]string{"chat", "PING"}, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "PONG") {
		t.Errorf("output %q should contain PONG", buf.String())
	}
}

func TestRun_ChatLLMError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "invalid api key"},
		})
	}))
	defer ts.Close()

	t.Setenv("NVIDIA_API_KEY", "bad-key")
	t.Setenv("NAVI_LLM_BASE_URL", ts.URL)

	err := run([]string{"chat", "hello"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("error %q should mention 'invalid api key'", err.Error())
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// noConfigPath returns a path that does not exist, causing LoadFrom to return defaults.
func noConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config.toml")
}

// writeCfg writes TOML content to a temp file and returns its path.
func writeCfg(t *testing.T, content string) string {
	t.Helper()
	path := noConfigPath(t)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeCfg: %v", err)
	}
	return path
}

func clearNaviEnvOverrides(t *testing.T) {
	t.Helper()
	t.Setenv("NAVI_DEFAULT_PROVIDER", "")
	t.Setenv("NAVI_DEFAULT_MODEL", "")
	t.Setenv("NAVI_DEFAULT_API_KEY_ENV", "")
	t.Setenv("NAVI_API_KEY", "")
}
