package tui

import (
	"context"
	"fmt"
	"navi/internal/core/ports"
	"navi/internal/core/services/agency"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type TUI struct {
	agency   *agency.Agency
	agentRepo ports.AgentRepository
}

func New(agency *agency.Agency, agentRepo ports.AgentRepository) *TUI {
	return &TUI{
		agency:    agency,
		agentRepo: agentRepo,
	}
}

func (t *TUI) Start() error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.render()
		case sig := <-sigCh:
			fmt.Printf("\nReceived %v, shutting down...\n", sig)
			return nil
		}
	}
}

func (t *TUI) render() {
	fmt.Print("\033[H\033[2J")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  NAVI - AI Orchestrator")
	fmt.Println(strings.Repeat("=", 60))

	agents, _ := t.agentRepo.FindAll(context.Background())
	if len(agents) == 0 {
		fmt.Println("\n  No agents registered")
	} else {
		for i, a := range agents {
			cfg := a.Config()
			status := "[trusted]"
			if !a.IsTrusted() {
				status = "[untrusted]"
			}
			fmt.Printf("  %d. %s %s\n", i+1, cfg.Name, status)
		}
	}

	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("  Agents:", len(agents), " | Press Ctrl+C to quit")
	fmt.Println(strings.Repeat("=", 60))
}
