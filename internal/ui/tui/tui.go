package tui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"navi/internal/core/domain"
	"navi/internal/core/ports"
	"navi/internal/core/services/orchestrator"
)

type TUI struct {
	orch      *orchestrator.Orchestrator
	agentRepo ports.AgentRepository
}

func New(orch *orchestrator.Orchestrator, agentRepo ports.AgentRepository) *TUI {
	return &TUI{orch: orch, agentRepo: agentRepo}
}

func (t *TUI) Start(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	t.render()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nShutting down...")
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
	width := 64

	line := func(s string) { fmt.Println(s) }
	sep := func() { line(strings.Repeat("─", width)) }
	header := func(s string) {
		pad := (width - len(s) - 2) / 2
		line(strings.Repeat("─", pad) + " " + s + " " + strings.Repeat("─", width-pad-len(s)-2))
	}

	header("NAVI Runtime Engine")
	line(fmt.Sprintf("  %-20s  %-10s  %-12s  %s", "AGENT", "ISOLATION", "PROVIDER", "CAPABILITIES"))
	sep()

	agents := t.orch.ListAgents()
	if len(agents) == 0 {
		dbAgents, _ := t.agentRepo.FindAll(context.Background())
		if len(dbAgents) == 0 {
			line("  No agents loaded. Run: navi agent create")
		} else {
			for _, a := range dbAgents {
				printAgentRow(a.Config())
			}
		}
	} else {
		for _, a := range agents {
			printAgentRow(a.Config())
		}
	}

	sep()
	line(fmt.Sprintf("  Agents: %d  |  %s  |  Ctrl+C to quit",
		len(agents), time.Now().Format("15:04:05")))
	sep()
	line("")
	line("  Commands:")
	line("    navi agent create         — create a new agent")
	line("    navi agent list           — list all agents")
	line("    navi agent remove <id>    — remove an agent")
	sep()
}

func printAgentRow(cfg domain.AgentConfig) {
	caps := make([]string, len(cfg.Capabilities))
	for i, c := range cfg.Capabilities {
		caps[i] = c.Raw()
	}
	capStr := strings.Join(caps, ", ")
	if len(capStr) > 28 {
		capStr = capStr[:25] + "..."
	}
	fmt.Printf("  %-20s  %-10s  %-12s  %s\n",
		cfg.ID, cfg.IsolationType, cfg.LLMProvider, capStr)
}
