// White-box tests for the config package.
// These live in package config (not config_test) so they can access the
// unexported userConfigDir seam and exercise the error branches that are
// impossible to reach without overriding the OS function.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper: replace the userConfigDir seam for the duration of the test and
// restore it via t.Cleanup.
func setUserConfigDir(t *testing.T, fn func() (string, error)) {
	t.Helper()
	orig := userConfigDir
	userConfigDir = fn
	t.Cleanup(func() { userConfigDir = orig })
}

// ── Dir ───────────────────────────────────────────────────────────────────────

func TestDir_Error_WhenUserConfigDirFails(t *testing.T) {
	sentinel := errors.New("userConfigDir: no home directory")
	setUserConfigDir(t, func() (string, error) {
		return "", sentinel
	})

	_, err := Dir()
	if err == nil {
		t.Fatal("expected error from Dir() when userConfigDir fails")
	}
	if !strings.Contains(err.Error(), "cannot determine user config directory") {
		t.Errorf("error %q should mention 'cannot determine user config directory'", err.Error())
	}
}

// ── Path ──────────────────────────────────────────────────────────────────────

func TestPath_Error_WhenDirFails(t *testing.T) {
	setUserConfigDir(t, func() (string, error) {
		return "", errors.New("no home directory")
	})

	_, err := Path()
	if err == nil {
		t.Fatal("expected error from Path() when Dir() fails")
	}
}

// ── Load ──────────────────────────────────────────────────────────────────────

// TestLoad_NoFile_ReturnsDefault verifies that Load() returns DefaultConfig
// when no config file exists at the platform path.
func TestLoad_NoFile_ReturnsDefault(t *testing.T) {
	// Redirect the platform config dir to an empty temp directory.
	tmpDir := t.TempDir()
	setUserConfigDir(t, func() (string, error) {
		return tmpDir, nil
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	// No file exists in tmpDir/navi/, so defaults should be returned.
	expect := DefaultConfig()
	if cfg.DefaultLLM.Provider != expect.DefaultLLM.Provider {
		t.Errorf("provider = %q, want %q", cfg.DefaultLLM.Provider, expect.DefaultLLM.Provider)
	}
	if cfg.DefaultLLM.APIKeyEnv != expect.DefaultLLM.APIKeyEnv {
		t.Errorf("api_key_env = %q, want %q", cfg.DefaultLLM.APIKeyEnv, expect.DefaultLLM.APIKeyEnv)
	}
}

// TestLoad_WithFile verifies that Load() parses the file at the platform path
// when one exists.
func TestLoad_WithFile(t *testing.T) {
	// Redirect to a temp dir and plant a config file at the expected location.
	tmpBase := t.TempDir()
	setUserConfigDir(t, func() (string, error) {
		return tmpBase, nil
	})

	// The actual path Load() will look at: <tmpBase>/navi/config.toml
	cfgDir := filepath.Join(tmpBase, "navi")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfgFile := filepath.Join(cfgDir, "config.toml")
	content := `[default_llm]
provider = "openai"
api_key_env = "OPENAI_API_KEY"
model = "gpt-4o"
`
	if err := os.WriteFile(cfgFile, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.DefaultLLM.Provider != ProviderOpenAI {
		t.Errorf("provider = %q, want %q", cfg.DefaultLLM.Provider, ProviderOpenAI)
	}
	if cfg.DefaultLLM.Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", cfg.DefaultLLM.Model)
	}
}

// TestLoad_Error_WhenDirFails verifies that Load() propagates the Dir() error.
func TestLoad_Error_WhenDirFails(t *testing.T) {
	setUserConfigDir(t, func() (string, error) {
		return "", errors.New("no home directory")
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error from Load() when Dir() fails")
	}
}

// ── validate (normalisation) ──────────────────────────────────────────────────

// TestValidate_NormalisesProviderToLowercase ensures that a provider written in
// uppercase in the TOML file is normalised before being returned to callers.
func TestValidate_NormalisesProviderToLowercase(t *testing.T) {
	cfg := Config{
		DefaultLLM: LLMConfig{Provider: "NVIDIA", APIKeyEnv: "X"},
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultLLM.Provider != ProviderNVIDIA {
		t.Errorf("provider = %q, want %q after normalisation", cfg.DefaultLLM.Provider, ProviderNVIDIA)
	}
}
