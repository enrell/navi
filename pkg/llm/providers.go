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
// Default model: meta/llama-3.1-8b-instruct.
func NVIDIA(apiKey string) openai.Config {
	return openai.Config{
		BaseURL: "https://integrate.api.nvidia.com/v1",
		APIKey:  apiKey,
		Model:   "meta/llama-3.1-8b-instruct",
	}
}

// OpenAI returns a Config for the official OpenAI API.
// Default model: gpt-4o-mini.
func OpenAI(apiKey string) openai.Config {
	return openai.Config{
		BaseURL: "https://api.openai.com/v1",
		APIKey:  apiKey,
		Model:   "gpt-4o-mini",
	}
}

// Groq returns a Config for Groq (OpenAI-compatible endpoint).
// Default model: llama-3.1-8b-instant.
func Groq(apiKey string) openai.Config {
	return openai.Config{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  apiKey,
		Model:   "llama-3.1-8b-instant",
	}
}

// OpenRouter returns a Config for OpenRouter.ai.
// Default model: meta-llama/llama-3.1-8b-instruct.
func OpenRouter(apiKey string) openai.Config {
	return openai.Config{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  apiKey,
		Model:   "meta-llama/llama-3.1-8b-instruct",
	}
}

// Ollama returns a Config for a local Ollama instance.
// Default model: llama3.1:8b.
func Ollama(model string) openai.Config {
	return openai.Config{
		BaseURL: "http://localhost:11434/v1",
		APIKey:  "ollama", // Ollama ignores the key but the header must be present
		Model:   model,
	}
}
