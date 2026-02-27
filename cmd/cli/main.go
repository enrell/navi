package main

import (
	"fmt"
	"navi/internal/agent"
	"navi/internal/orchestrator"
	"navi/internal/tui"
	"os"
)

func main() {
	reg := agent.NewInMemoryAgentRegistry()
	orch := orchestrator.NewSimpleOrchestrator(reg, nil)

	tui := tui.NewTUI(orch)
	fmt.Println("Starting Navi TUI...")
	if err := tui.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}
