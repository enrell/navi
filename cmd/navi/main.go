package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"navi/internal/adapters/isolation/bubblewrap"
	"navi/internal/adapters/isolation/docker"
	"navi/internal/adapters/isolation/native"
	"navi/internal/adapters/llm/openai"
	"navi/internal/adapters/registry/localfs"
	"navi/internal/adapters/storage/sqlite"
	"navi/internal/core/domain"
	"navi/internal/core/ports"
	"navi/internal/core/services/orchestrator"
	"navi/internal/ui/tui"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ── Storage ────────────────────────────────────────────────────────────────
	home, err := os.UserHomeDir()
	if err != nil {
		fatalf("cannot get home dir: %v", err)
	}

	dbPath := filepath.Join(home, ".navi", "navi.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		fatalf("cannot create db dir: %v", err)
	}

	sqliteRepo, err := sqlite.NewSQLiteRepository("file:" + dbPath + "?cache=shared&_journal_mode=WAL")
	if err != nil {
		fatalf("cannot init sqlite: %v", err)
	}

	// ── Config Registry ────────────────────────────────────────────────────────
	agentsDir := filepath.Join(home, ".config", "navi", "agents")
	cfgReg, err := localfs.New(agentsDir)
	if err != nil {
		fatalf("cannot init config registry: %v", err)
	}

	// ── Adapter Factories ──────────────────────────────────────────────────────
	llmFactory := func(cfg domain.AgentConfig) (domain.LLMPort, error) {
		apiKey := cfg.LLMAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		switch strings.ToLower(cfg.LLMProvider) {
		case "openai", "":
			return openai.New(apiKey, cfg.LLMModel, cfg.LLMBaseURL, cfg.LLMTemperature, cfg.LLMMaxTokens), nil
		default:
			// Fallback: treat as OpenAI-compatible endpoint (Ollama, Together, etc.)
			return openai.New(apiKey, cfg.LLMModel, cfg.LLMBaseURL, cfg.LLMTemperature, cfg.LLMMaxTokens), nil
		}
	}

	isoFactory := func(cfg domain.AgentConfig) (domain.IsolationPort, error) {
		workspaceDir := filepath.Join(home, ".navi", "workspace", cfg.ID)
		_ = os.MkdirAll(workspaceDir, 0755)
		switch strings.ToLower(cfg.IsolationType) {
		case "docker":
			image := "ubuntu:22.04"
			if v, ok := cfg.IsolationConfig["image"]; ok {
				image = v
			}
			return docker.New(image, workspaceDir), nil
		case "bubblewrap", "bwrap":
			return bubblewrap.New(workspaceDir), nil
		default: // "native"
			allowedPaths := []string{workspaceDir}
			return native.New(allowedPaths), nil
		}
	}

	// ── Orchestrator ───────────────────────────────────────────────────────────
	orch := orchestrator.New(cfgReg, sqliteRepo, llmFactory, isoFactory)

	// ── CLI Dispatch ───────────────────────────────────────────────────────────
	args := os.Args[1:]

	if len(args) >= 2 && args[0] == "agent" && args[1] == "create" {
		runAgentCreate(ctx, orch)
		return
	}

	if len(args) >= 2 && args[0] == "agent" && args[1] == "list" {
		runAgentList(ctx, sqliteRepo)
		return
	}

	if len(args) >= 3 && args[0] == "agent" && args[1] == "remove" {
		runAgentRemove(ctx, orch, domain.AgentID(args[2]))
		return
	}

	// Default: start TUI
	if err := orch.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] orchestrator start: %v\n", err)
	}
	defer orch.Shutdown()

	t := tui.New(orch, sqliteRepo)
	fmt.Println("Starting Navi — press Ctrl+C to quit.")
	if err := t.Start(ctx); err != nil {
		fatalf("TUI error: %v", err)
	}
}

// ─── CLI Commands ─────────────────────────────────────────────────────────────

func runAgentCreate(ctx context.Context, orch *orchestrator.Orchestrator) {
	sc := bufio.NewScanner(os.Stdin)

	prompt := func(label, defaultVal string) string {
		if defaultVal != "" {
			fmt.Printf("%s [%s]: ", label, defaultVal)
		} else {
			fmt.Printf("%s: ", label)
		}
		sc.Scan()
		v := strings.TrimSpace(sc.Text())
		if v == "" {
			return defaultVal
		}
		return v
	}

	fmt.Println("\n=== navi agent create ===")
	id := prompt("Agent ID (e.g. researcher)", "")
	if id == "" {
		fatalf("agent ID is required")
	}
	description := prompt("Description", "")
	llmProvider := prompt("LLM provider (openai/ollama)", "openai")
	llmModel := prompt("LLM model", "gpt-4o-mini")
	llmAPIKey := prompt("LLM API key (or set OPENAI_API_KEY env)", "")
	isolation := prompt("Isolation (native/docker/bubblewrap)", "native")

	fmt.Println("\nCapabilities (one per line, empty line to finish):")
	fmt.Println("  Examples: filesystem:workspace:rw  exec:bash,git  network:api.github.com:443")
	var rawCaps []string
	for {
		fmt.Print("  > ")
		sc.Scan()
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			break
		}
		rawCaps = append(rawCaps, line)
	}

	fmt.Println("\nSystem prompt (AGENT.md content). Type END on a new line when done:")
	var promptLines []string
	for {
		sc.Scan()
		line := sc.Text()
		if line == "END" {
			break
		}
		promptLines = append(promptLines, line)
	}
	systemPrompt := strings.Join(promptLines, "\n")

	caps := make([]domain.Capability, 0, len(rawCaps))
	for _, raw := range rawCaps {
		c, err := domain.ParseCapability(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] skipping invalid capability %q: %v\n", raw, err)
			continue
		}
		caps = append(caps, c)
	}

	cfg := domain.AgentConfig{
		ID:             id,
		Name:           id,
		Description:    description,
		Type:           "generic",
		PromptFile:     "AGENT.md",
		SystemPrompt:   systemPrompt,
		LLMProvider:    llmProvider,
		LLMModel:       llmModel,
		LLMAPIKey:      llmAPIKey,
		IsolationType:  isolation,
		Capabilities:   caps,
		Timeout:        30 * time.Minute,
		MaxConcurrent:  5,
		LLMTemperature: 0.7,
		LLMMaxTokens:   4096,
	}

	if err := orch.RegisterAgent(ctx, cfg); err != nil {
		fatalf("failed to register agent: %v", err)
	}
	fmt.Printf("\n✓ Agent %q created and registered.\n", id)
}

func runAgentList(_ context.Context, repo ports.AgentRepository) {
	agents, err := repo.FindAll(context.Background())
	if err != nil {
		fatalf("list agents: %v", err)
	}
	if len(agents) == 0 {
		fmt.Println("No agents registered.")
		return
	}
	fmt.Printf("%-20s  %-12s  %-10s  %s\n", "ID", "PROVIDER", "ISOLATION", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 70))
	for _, a := range agents {
		cfg := a.Config()
		fmt.Printf("%-20s  %-12s  %-10s  %s\n", cfg.ID, cfg.LLMProvider, cfg.IsolationType, cfg.Description)
	}
}

func runAgentRemove(ctx context.Context, orch *orchestrator.Orchestrator, id domain.AgentID) {
	if err := orch.RemoveAgent(ctx, id); err != nil {
		fatalf("remove agent: %v", err)
	}
	fmt.Printf("✓ Agent %q removed.\n", id)
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "navi: "+format+"\n", a...)
	os.Exit(1)
}
