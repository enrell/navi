// Package llm provides provider presets for OpenAI-compatible APIs.
//
// Every provider here is just a named Config pointing at a different BaseURL.
// They all share the single generic openai.Client — so a change in the OpenAI
// wire format ripples automatically to every provider, and any provider that
// diverges from the spec will be caught by the shared integration tests.
package llm

import "navi/pkg/llm/openai"

// NVIDIA returns a Config for NVIDIA NIM (api.nvidia.com).
// Set the NVIDIA_API_KEY environment variable to authenticate.
func NVIDIA(apiKey string) openai.Config {
	return openai.Config{
		BaseURL: "https://integrate.api.nvidia.com/v1",
		APIKey:  apiKey,
	}
}

// OpenAI returns a Config for the official OpenAI API.
func OpenAI(apiKey string) openai.Config {
	return openai.Config{
		BaseURL: "https://api.openai.com/v1",
		APIKey:  apiKey,
	}
}

// Groq returns a Config for Groq (OpenAI-compatible endpoint).
func Groq(apiKey string) openai.Config {
	return openai.Config{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  apiKey,
	}
}

// OpenRouter returns a Config for OpenRouter.ai.
func OpenRouter(apiKey string) openai.Config {
	return openai.Config{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  apiKey,
	}
}

// Ollama returns a Config for a local Ollama instance.
func Ollama(model string) openai.Config {
	return openai.Config{
		BaseURL: "http://localhost:11434/v1",
		APIKey:  "ollama", // Ollama ignores the key but the header must be present
		Model:   model,
	}
}
