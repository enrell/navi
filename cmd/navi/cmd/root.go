// Package cmd defines the Navi CLI using cobra.
//
// Dependency injection pattern: main.go wires all services/adapters and passes
// them into NewRootCommand. Commands never import adapters or read env vars
// directly — that's the wiring layer's job (main.go).
package cmd

import (
	"io"

	"github.com/spf13/cobra"

	agentsvc "navi/internal/core/services/agent"
	"navi/internal/core/services/chat"
	orchestratorsvc "navi/internal/core/services/orchestrator"
	tasksvc "navi/internal/core/services/task"
)

// Dependencies carries the wired application services into the command tree.
// Adding a new service? Add a field here — commands stay clean.
type Dependencies struct {
	Chat   *chat.Service
	Tasks  *tasksvc.Service
	Agents *agentsvc.Service
	Orchestrator *orchestratorsvc.Service
}

// NewRootCommand builds and returns the fully configured cobra command tree.
// out is the writer for all command output — set to os.Stdout in production,
// or a *bytes.Buffer in tests.
func NewRootCommand(deps Dependencies, out io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "navi",
		Short: "Navi — secure AI orchestrator",
		Long: `Navi is a secure AI orchestrator built with hexagonal architecture.
Agents are defined by config files, not hardcoded.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.SetOut(out)
	root.SetErr(out)

	root.AddCommand(newChatCommand(deps, out))
	root.AddCommand(newReplCommand(deps, out))
	root.AddCommand(newServeCommand(deps, out))

	return root
}
