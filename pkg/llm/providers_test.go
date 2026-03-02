package llm_test

import (
	"strings"
	"testing"

	"navi/pkg/llm"
)

// Each provider must use a distinct, correct base URL and carry the API key.
// If a provider accidentally reuses another provider's endpoint, these tests catch it.

func TestNVIDIA_Config(t *testing.T) {
	cfg := llm.NVIDIA("key-nvidia")
	assertConfig(t, cfg.BaseURL, cfg.APIKey, "key-nvidia",
		"integrate.api.nvidia.com", "meta/llama")
}

func TestOpenAI_Config(t *testing.T) {
	cfg := llm.OpenAI("key-openai")
	assertConfig(t, cfg.BaseURL, cfg.APIKey, "key-openai",
		"api.openai.com", "gpt-")
}

func TestGroq_Config(t *testing.T) {
	cfg := llm.Groq("key-groq")
	assertConfig(t, cfg.BaseURL, cfg.APIKey, "key-groq",
		"api.groq.com", "llama")
}

func TestOpenRouter_Config(t *testing.T) {
	cfg := llm.OpenRouter("key-openrouter")
	assertConfig(t, cfg.BaseURL, cfg.APIKey, "key-openrouter",
		"openrouter.ai", "meta-llama")
}

func TestOllama_Config(t *testing.T) {
	cfg := llm.Ollama("mistral:latest")
	assertConfig(t, cfg.BaseURL, cfg.APIKey, "ollama",
		"localhost:11434", "mistral")
}

// Each provider base URL must be distinct — no two providers should share an endpoint.
func TestProviders_DistinctBaseURLs(t *testing.T) {
	configs := map[string]string{
		"NVIDIA":     llm.NVIDIA("k").BaseURL,
		"OpenAI":     llm.OpenAI("k").BaseURL,
		"Groq":       llm.Groq("k").BaseURL,
		"OpenRouter": llm.OpenRouter("k").BaseURL,
		"Ollama":     llm.Ollama("model").BaseURL,
	}

	seen := map[string]string{}
	for name, url := range configs {
		if prev, ok := seen[url]; ok {
			t.Errorf("providers %s and %s share the same base URL: %s", prev, name, url)
		}
		seen[url] = name
	}
}

func assertConfig(t *testing.T, gotURL, gotKey, wantKey, urlFragment, modelFragment string) {
	t.Helper()
	if !strings.Contains(gotURL, urlFragment) {
		t.Errorf("BaseURL %q does not contain %q", gotURL, urlFragment)
	}
	if gotKey != wantKey {
		t.Errorf("APIKey = %q, want %q", gotKey, wantKey)
	}
}
