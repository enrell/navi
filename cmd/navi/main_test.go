package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"navi/internal/config"
	"navi/internal/core/domain"
	"navi/internal/core/ports"
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

type agentCallLLMStub struct {
	replies  []string
	err      error
	messages []domain.Message
	idx      int
}

func (s *agentCallLLMStub) Chat(_ context.Context, messages []domain.Message) (string, error) {
	s.messages = append([]domain.Message(nil), messages...)
	if s.err != nil {
		return "", s.err
	}
	if s.idx >= len(s.replies) {
		return "", nil
	}
	out := s.replies[s.idx]
	s.idx++
	return out, nil
}

var _ ports.LLMPort = (*agentCallLLMStub)(nil)

type agentCallToolExecStub struct {
	tools    []ports.Tool
	result   string
	err      error
	lastName string
	lastIn   string
}

func (s *agentCallToolExecStub) ListTools(context.Context) ([]ports.Tool, error) {
	return s.tools, nil
}

func (s *agentCallToolExecStub) ExecuteTool(_ context.Context, name, input string) (string, error) {
	s.lastName = name
	s.lastIn = input
	if s.err != nil {
		return "", s.err
	}
	return s.result, nil
}

var _ ports.ToolExecutor = (*agentCallToolExecStub)(nil)

func TestParseAgentCallInput_JSON(t *testing.T) {
	agentID, prompt, err := parseAgentCallInput(`{"agent_id":"coder","prompt":"fix bug"}`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if agentID != "coder" || prompt != "fix bug" {
		t.Fatalf("got (%q,%q), want (coder,fix bug)", agentID, prompt)
	}
}

func TestParseAgentCallInput_ColonFormat(t *testing.T) {
	agentID, prompt, err := parseAgentCallInput("tester: run tests")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if agentID != "tester" || prompt != "run tests" {
		t.Fatalf("got (%q,%q), want (tester,run tests)", agentID, prompt)
	}
}

func TestBuildAgentDelegationTool_CallsSelectedAgent(t *testing.T) {
	llm := &agentCallLLMStub{replies: []string{"done"}}
	tools := &agentCallToolExecStub{tools: []ports.Tool{{Name: "native.echo", Description: "echo"}}}
	coder, err := domain.NewGenericAgent(domain.AgentConfig{ID: "coder", Type: domain.AgentTypeGeneric, Name: "Coder"}, "You are coder")
	if err != nil {
		t.Fatalf("NewGenericAgent: %v", err)
	}
	tool := buildAgentDelegationTool(llm, tools, map[string]*domain.GenericAgent{"coder": coder})

	got, err := tool(context.Background(), `{"agent_id":"coder","prompt":"implement feature"}`)
	if err != nil {
		t.Fatalf("tool error: %v", err)
	}
	if got != "done" {
		t.Fatalf("got %q, want done", got)
	}
	if len(llm.messages) == 0 || llm.messages[0].Role != domain.RoleSystem {
		t.Fatalf("unexpected messages sent to llm: %+v", llm.messages)
	}
}

func TestBuildAgentDelegationTool_SpecialistCanCallTool(t *testing.T) {
	llm := &agentCallLLMStub{replies: []string{
		`TOOL_CALL {"name":"native.echo","input":"hello from specialist"}`,
		"specialist final answer",
	}}
	tools := &agentCallToolExecStub{
		tools:  []ports.Tool{{Name: "native.echo", Description: "echo"}, {Name: "agent.call", Description: "delegate"}},
		result: "echo-result",
	}
	researcher, err := domain.NewGenericAgent(domain.AgentConfig{ID: "researcher", Type: domain.AgentTypeGeneric, Name: "Researcher"}, "You are researcher")
	if err != nil {
		t.Fatalf("NewGenericAgent: %v", err)
	}
	tool := buildAgentDelegationTool(llm, tools, map[string]*domain.GenericAgent{"researcher": researcher})

	got, err := tool(context.Background(), `{"agent_id":"researcher","prompt":"use tools"}`)
	if err != nil {
		t.Fatalf("tool error: %v", err)
	}
	if got != "specialist final answer" {
		t.Fatalf("got %q, want specialist final answer", got)
	}
	if tools.lastName != "native.echo" || tools.lastIn != "hello from specialist" {
		t.Fatalf("tool execute = (%q,%q), want (native.echo,hello from specialist)", tools.lastName, tools.lastIn)
	}
}

func TestParseAgentToolCalls_ObjectInput(t *testing.T) {
	calls, _, ok := parseAgentToolCalls(`TOOL_CALL {"name":"agent.call","input":{"agent_id":"researcher","prompt":"list dirs"}}`)
	if !ok {
		t.Fatal("expected tool call parse success")
	}
	if len(calls) != 1 {
		t.Fatalf("calls len = %d, want 1", len(calls))
	}
	if !strings.Contains(calls[0].Input, `"agent_id":"researcher"`) {
		t.Fatalf("input = %q, want JSON object encoded string", calls[0].Input)
	}
}

func TestParseAgentToolCalls_XMLFormat(t *testing.T) {
	reply := "I'll call the tool.\n<tool_call>\n<function=native.list_dirs>\n<parameter=path>docs</parameter>\n</function>\n</tool_call>"
	calls, thinking, ok := parseAgentToolCalls(reply)
	if !ok {
		t.Fatal("expected XML tool call parse success")
	}
	if len(calls) != 1 {
		t.Fatalf("calls len = %d, want 1", len(calls))
	}
	if calls[0].Name != "native.list_dirs" {
		t.Fatalf("name = %q, want native.list_dirs", calls[0].Name)
	}
	if calls[0].Input != "docs" {
		t.Fatalf("input = %q, want docs", calls[0].Input)
	}
	if thinking != "I'll call the tool." {
		t.Fatalf("thinking = %q, want prefix text", thinking)
	}
}

func TestParseAgentToolCalls_XMLMultipleParams(t *testing.T) {
	reply := `<tool_call>
<function=native.list_dirs>
<parameter=path>src</parameter>
<parameter=limit>5</parameter>
</function>
</tool_call>`
	calls, _, ok := parseAgentToolCalls(reply)
	if !ok {
		t.Fatal("expected parse success")
	}
	if len(calls) != 1 {
		t.Fatalf("calls len = %d, want 1", len(calls))
	}
	if calls[0].Name != "native.list_dirs" {
		t.Fatalf("name = %q, want native.list_dirs", calls[0].Name)
	}
	// Multiple params → JSON encoded
	if !strings.Contains(calls[0].Input, `"path":"src"`) || !strings.Contains(calls[0].Input, `"limit":"5"`) {
		t.Fatalf("input = %q, want JSON with path and limit", calls[0].Input)
	}
}

func TestParseXMLToolCalls_NoXML(t *testing.T) {
	_, _, ok := parseXMLToolCalls("just a regular response")
	if ok {
		t.Fatal("expected no match for regular text")
	}
}

func TestBuildAgentDelegationTool_XMLToolCallExecuted(t *testing.T) {
	llm := &agentCallLLMStub{replies: []string{
		"<tool_call>\n<function=native.list_dirs>\n<parameter=path>.</parameter>\n</function>\n</tool_call>",
		"found 2 directories",
	}}
	tools := &agentCallToolExecStub{
		tools:  []ports.Tool{{Name: "native.list_dirs", Description: "list dirs"}},
		result: `{"path":".","count":2,"directories":["cmd","internal"]}`,
	}
	researcher, err := domain.NewGenericAgent(domain.AgentConfig{ID: "researcher", Type: domain.AgentTypeGeneric, Name: "Researcher"}, "You are researcher")
	if err != nil {
		t.Fatalf("NewGenericAgent: %v", err)
	}
	tool := buildAgentDelegationTool(llm, tools, map[string]*domain.GenericAgent{"researcher": researcher})

	got, err := tool(context.Background(), `{"agent_id":"researcher","prompt":"list dirs"}`)
	if err != nil {
		t.Fatalf("tool error: %v", err)
	}
	if got != "found 2 directories" {
		t.Fatalf("got %q, want found 2 directories", got)
	}
	if tools.lastName != "native.list_dirs" {
		t.Fatalf("tool name = %q, want native.list_dirs", tools.lastName)
	}
}

func TestIsSpecialistAgentID(t *testing.T) {
	for _, id := range []string{"planner", "researcher", "coder", "tester", "CODER"} {
		if !isSpecialistAgentID(id) {
			t.Fatalf("expected %q to be specialist", id)
		}
	}
	for _, id := range []string{"orchestrator", "", "foo"} {
		if isSpecialistAgentID(id) {
			t.Fatalf("expected %q not to be specialist", id)
		}
	}
}
