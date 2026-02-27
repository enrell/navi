package main

import (
	"context"
	"fmt"
	"navi/internal/adapters/storage/sqlite"
	"navi/internal/core/domain"
	"navi/internal/core/ports"
	"navi/internal/core/services/agency"
	"navi/internal/ui/tui"
	"os"
	"path/filepath"
)

func main() {
	ctx := context.Background()

	// Init SQLite repository
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get home dir: %v\n", err)
		os.Exit(1)
	}
	dbPath := filepath.Join(home, ".navi", "navi.db")
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "cannot create db dir: %v\n", err)
		os.Exit(1)
	}
	sqliteRepo, err := sqlite.NewSQLiteRepository("file:" + dbPath + "?cache=shared&_journal_mode=WAL")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot init sqlite: %v\n", err)
		os.Exit(1)
	}

	// For now, agentRepo is just the sqliteRepo (implements both EventRepository and AgentRepository interfaces)
	var agentRepo ports.AgentRepository = sqliteRepo

	// Create agency
	agencySvc := agency.NewAgency(agentRepo, sqliteRepo)

	// Register a default agent if none exist
	count, _ := agentRepo.FindAll(ctx)
	if len(count) == 0 {
		defaultCfg := domain.AgentConfig{
			Name:          "orchestrator",
			SystemPrompt: "You are Navi, the orchestrator agent.",
			IsolationType: "native",
			LLMProvider:   "openai",
			LLMModel:      "gpt-4o",
		}
		if err := agencySvc.RegisterAgent(ctx, defaultCfg); err != nil {
			fmt.Fprintf(os.Stderr, "cannot register default agent: %v\n", err)
		}
	}

	// Start TUI
	tui := tui.New(agencySvc, agentRepo)
	fmt.Println("Starting Navi TUI...")
	if err := tui.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}
