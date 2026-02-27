package tui

import (
	"context"
	"fmt"
	"navi/internal/orchestrator"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type TUI struct {
	orch   *orchestrator.SimpleOrchestrator
	ctx    context.Context
	cancel context.CancelFunc
}

func NewTUI(orch *orchestrator.SimpleOrchestrator) *TUI {
	ctx, cancel := context.WithCancel(context.Background())
	return &TUI{
		orch:   orch,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (t *TUI) Start() error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return nil
		case sig := <-sigCh:
			fmt.Printf("\nReceived %v, shutting down...\n", sig)
			return nil
		case <-ticker.C:
			t.render()
		}
	}
}

func (t *TUI) render() {
	fmt.Print("\033[H\033[2J")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  NAVI - AI Orchestrator")
	fmt.Println(strings.Repeat("=", 60))

	agents := t.orch.ListAgents()
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
