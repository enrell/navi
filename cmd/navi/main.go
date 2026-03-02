// Navi — secure AI orchestrator.
//
// This file is the composition root: it reads environment/config, wires all
// adapters and services together, then hands off to cobra.
//
// Keep it thin. Business logic belongs in internal/core/services/.
// Adapter choices belong in internal/adapters/.
package main

import (
	"fmt"
	"io"
	"os"

	navcmd "navi/cmd/navi/cmd"
	llmadapter "navi/internal/adapters/llm/openai"
	"navi/internal/core/services/chat"
	"navi/pkg/llm"
	pkgopenai "navi/pkg/llm/openai"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run is the testable entry point: it wires all dependencies and executes the
// cobra command tree. Keeping args and out as parameters makes it injectable.
func run(args []string, out io.Writer) error {
	cfg, err := buildLLMConfig()
	if err != nil {
		return err
	}

	// Wire: pkg HTTP client → adapter (satisfies LLMPort) → chat service
	adapter := llmadapter.New(cfg)
	chatService := chat.New(adapter)

	deps := navcmd.Dependencies{
		Chat: chatService,
	}

	root := navcmd.NewRootCommand(deps, out)
	root.SetArgs(args)
	return root.Execute()
}

// buildLLMConfig reads environment variables and returns the active LLM config.
// Default provider: NVIDIA NIM (NVIDIA_API_KEY).
// Override the base URL for testing: NAVI_LLM_BASE_URL.
func buildLLMConfig() (pkgopenai.Config, error) {
	apiKey := os.Getenv("NVIDIA_API_KEY")
	if apiKey == "" {
		return pkgopenai.Config{}, fmt.Errorf(
			"NVIDIA_API_KEY environment variable is not set\n" +
				"  export NVIDIA_API_KEY=<your-key>  # https://build.nvidia.com",
		)
	}

	cfg := llm.NVIDIA(apiKey)

	// NAVI_LLM_BASE_URL overrides the provider endpoint — used in tests and
	// local development to point at an httptest server or a local proxy.
	if override := os.Getenv("NAVI_LLM_BASE_URL"); override != "" {
		cfg.BaseURL = override
	}

	return cfg, nil
}
