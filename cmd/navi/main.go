package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
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
	"navi/internal/ui/api"
	"navi/internal/ui/repl"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	home, err := os.UserHomeDir()
	if err != nil {
		fatalf("cannot get home dir: %v", err)
	}

	dbPath := filepath.Join(home, ".navi", "navi.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		fatalf("cannot create db dir: %v", err)
	}

	sqliteRepo, err := sqlite.NewSQLiteRepository(dbPath)
	if err != nil {
		fatalf("cannot init sqlite: %v", err)
	}

	agentsDir := filepath.Join(home, ".config", "navi", "agents")
	cfgReg, err := localfs.New(agentsDir)
	if err != nil {
		fatalf("cannot init config registry: %v", err)
	}

	llmFactory := func(cfg domain.AgentConfig) (domain.LLMPort, error) {
		apiKey := cfg.LLMAPIKey
		baseURL := cfg.LLMBaseURL
		if apiKey == "" {
			if key := os.Getenv("OPENAI_API_KEY"); key != "" {
				apiKey = key
			} else if key := os.Getenv("NVIDIA_API_KEY"); key != "" {
				apiKey = key
				if baseURL == "" {
					baseURL = "https://integrate.api.nvidia.com/v1"
				}
			} else if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
				apiKey = key
				if baseURL == "" {
					baseURL = "https://openrouter.ai/api/v1"
				}
			} else if key := os.Getenv("LLM_API_KEY"); key != "" {
				apiKey = key
			}
		}

		if b := os.Getenv("LLM_BASE_URL"); b != "" {
			baseURL = b
		}

		switch strings.ToLower(cfg.LLMProvider) {
		case "openai", "":
			adapter, err := openai.New(apiKey, cfg.LLMModel, baseURL, cfg.LLMTemperature, cfg.LLMMaxTokens)
			if err != nil {
				return nil, fmt.Errorf("failed to create OpenAI adapter: %w", err)
			}
			return adapter, nil
		default:
			adapter, err := openai.New(apiKey, cfg.LLMModel, baseURL, cfg.LLMTemperature, cfg.LLMMaxTokens)
			if err != nil {
				return nil, fmt.Errorf("failed to create OpenAI adapter: %w", err)
			}
			return adapter, nil
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
		default:
			allowedPaths := []string{workspaceDir}
			return native.New(allowedPaths), nil
		}
	}

	orch := orchestrator.New(cfgReg, sqliteRepo, llmFactory, isoFactory)

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

	if len(args) >= 1 && args[0] == "serve" {
		runServe(ctx, orch, args[1:])
		return
	}

	if len(args) >= 1 && args[0] == "repl" {
		runREPL(ctx, dbPath)
		return
	}

	if len(args) >= 2 && args[0] == "chat" {
		runChat(ctx, strings.Join(args[1:], " "))
		return
	}

	if err := orch.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] orchestrator start: %v\n", err)
	}
	defer orch.Shutdown()

	// Default: start REST API server
	fmt.Println("Starting Navi REST API server on http://localhost:8080")
	fmt.Println("Press Ctrl+C to quit.")
	fmt.Println("")
	fmt.Println("Available commands:")
	fmt.Println("  navi serve        - Start REST API server")
	fmt.Println("  navi repl         - Start REPL")
	fmt.Println("  navi chat <msg>  - Single chat message")
	fmt.Println("  navi agent list   - List agents")

	server := api.New()
	handlers := api.NewHandlers(orch)
	handlers.RegisterRoutes(server.Router())

	if err := http.ListenAndServe(":8080", server); err != nil {
		fatalf("server error: %v", err)
	}
}

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

func runREPL(ctx context.Context, dbPath string) {
	var apiKey, model, baseURL string

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		apiKey = key
		model = "gpt-4o-mini"
	} else if key := os.Getenv("NVIDIA_API_KEY"); key != "" {
		apiKey = key
		baseURL = "https://integrate.api.nvidia.com/v1"
		model = "meta/llama-3.3-70b-instruct" // Default nvidia model
	} else if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
		apiKey = key
		baseURL = "https://openrouter.ai/api/v1"
		model = "meta-llama/llama-3-8b-instruct:free" // Default free openrouter model
	} else if key := os.Getenv("LLM_API_KEY"); key != "" {
		apiKey = key
	}

	// Any specific overrides from env
	if m := os.Getenv("LLM_MODEL"); m != "" {
		model = m
	}
	if b := os.Getenv("LLM_BASE_URL"); b != "" {
		baseURL = b
	}

	if apiKey == "" {
		fatalf("No API key found. Please set OPENAI_API_KEY, NVIDIA_API_KEY, or OPENROUTER_API_KEY")
	}

	r, err := repl.NewRepl(dbPath, apiKey, model, baseURL)
	if err != nil {
		fatalf("Failed to initialize REPL: %v", err)
	}
	defer r.Close()

	if err := r.Run(ctx); err != nil {
		fatalf("REPL error: %v", err)
	}
}

func runChat(ctx context.Context, prompt string) {
	var apiKey, model, baseURL string

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		apiKey = key
		model = "gpt-4o-mini"
	} else if key := os.Getenv("NVIDIA_API_KEY"); key != "" {
		apiKey = key
		baseURL = "https://integrate.api.nvidia.com/v1"
		model = "meta/llama-3.3-70b-instruct"
	} else if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
		apiKey = key
		baseURL = "https://openrouter.ai/api/v1"
		model = "meta-llama/llama-3-8b-instruct:free"
	} else if key := os.Getenv("LLM_API_KEY"); key != "" {
		apiKey = key
	}

	if m := os.Getenv("LLM_MODEL"); m != "" {
		model = m
	}
	if b := os.Getenv("LLM_BASE_URL"); b != "" {
		baseURL = b
	}

	if apiKey == "" {
		fatalf("No API key found. Please set OPENAI_API_KEY, NVIDIA_API_KEY, or OPENROUTER_API_KEY")
	}

	llm, err := openai.New(apiKey, model, baseURL, 0.7, 4096)
	if err != nil {
		fatalf("Failed to create LLM adapter: %v", err)
	}
	cfg := domain.AgentConfig{
		ID:           "cli-agent",
		Name:         "CLI Chat",
		SystemPrompt: "You are a helpful CLI assistant.",
	}
	agent := domain.NewGenericAgent(cfg, llm, nil, nil)

	task := domain.Task{
		ID:      "cli-chat",
		Prompt:  prompt,
		AgentID: agent.ID(),
	}

	result, err := agent.Execute(ctx, task)
	if err != nil {
		fatalf("Agent Error: %v", err)
	}
	fmt.Println(result.Output)
}

func runServe(ctx context.Context, orch *orchestrator.Orchestrator, args []string) {
	// Parse flags
	addr := ":8080"
	for i := 0; i < len(args); i++ {
		if args[i] == "--port" || args[i] == "-p" {
			if i+1 < len(args) {
				addr = ":" + args[i+1]
				i++
			}
		} else if args[i] == "--host" {
			if i+1 < len(args) {
				addr = args[i+1] + addr
				i++
			}
		}
	}

	// Start orchestrator if not already started
	if err := orch.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] orchestrator start: %v\n", err)
	}
	defer orch.Shutdown()

	// Create API server
	server := api.New()
	handlers := api.NewHandlers(orch)
	handlers.RegisterRoutes(server.Router())

	fmt.Printf("Starting REST API server on http://%s\n", addr)
	fmt.Println("Endpoints:")
	fmt.Println("  GET  /health           - Health check")
	fmt.Println("  GET  /agents           - List agents")
	fmt.Println("  GET  /agents/:id       - Get agent details")
	fmt.Println("  POST /tasks            - Create task")
	fmt.Println("  GET  /tasks            - List tasks")
	fmt.Println("  GET  /tasks/:id       - Get task status")

	if err := http.ListenAndServe(addr, server); err != nil {
		fatalf("server error: %v", err)
	}
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "navi: "+format+"\n", a...)
	os.Exit(1)
}
