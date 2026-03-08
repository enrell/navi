package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newAgentsCommand(deps Dependencies, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Inspect and sync registered agents",
	}

	cmd.AddCommand(
		newAgentsListCommand(deps, out),
		newAgentsGetCommand(deps, out),
		newAgentsSyncCommand(deps, out),
	)

	return cmd
}

func newAgentsListCommand(deps Dependencies, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			agentService, _, cleanup, err := ensureAgentService(cmd.Context(), deps)
			if err != nil {
				return err
			}
			defer cleanup()

			agents, err := agentService.List(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(out, agents)
		},
	}
}

func newAgentsGetCommand(deps Dependencies, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one registered agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentService, _, cleanup, err := ensureAgentService(cmd.Context(), deps)
			if err != nil {
				return err
			}
			defer cleanup()

			agent, err := agentService.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(out, agent)
		},
	}
}

func newAgentsSyncCommand(deps Dependencies, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync agents from local agent roots into SQLite",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, syncer, cleanup, err := ensureAgentService(cmd.Context(), deps)
			if err != nil {
				return err
			}
			defer cleanup()

			if syncer == nil {
				return fmt.Errorf("agents: sync is unavailable for the current agent service")
			}

			n, err := syncer.Sync(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(out, map[string]any{
				"synced":  n,
				"message": "agents synced",
			})
		},
	}
}
