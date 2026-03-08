package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	navitui "navi/internal/tui"
)

func newTUICommand(deps Dependencies, out io.Writer) *cobra.Command {
	var plain bool

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Start the Bubble Tea terminal UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInteractiveSession(cmd, deps, out, plain, true)
		},
	}

	cmd.Flags().BoolVar(&plain, "plain", false, "Force the line-based REPL instead of the full-screen TUI")
	return cmd
}

func runInteractiveSession(cmd *cobra.Command, deps Dependencies, out io.Writer, plain bool, preferTUI bool) error {
	if deps.Orchestrator == nil && deps.Chat == nil {
		return fmt.Errorf("repl: neither orchestrator nor chat service is wired")
	}

	if !plain && preferTUI && navitui.IsInteractiveTTY(cmd.InOrStdin(), out) {
		var orchestrator navitui.Orchestrator
		if deps.Orchestrator != nil {
			orchestrator = deps.Orchestrator
		}
		var chat navitui.Chatter
		if deps.Chat != nil {
			chat = deps.Chat
		}
		var agents navitui.AgentLister
		if deps.Agents != nil {
			agents = deps.Agents
		}

		return navitui.Run(cmd.Context(), cmd.InOrStdin(), out, navitui.Services{
			Orchestrator: orchestrator,
			Chat:         chat,
			Agents:       agents,
			ModelName:    deps.ModelName,
			WorkDir:      deps.WorkDir,
			ContextLimit: deps.ContextLimit,
		})
	}

	return runPlainRepl(cmd, deps, out)
}
