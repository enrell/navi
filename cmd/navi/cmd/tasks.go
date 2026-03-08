package cmd

import (
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func newTasksCommand(deps Dependencies, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "Create and inspect local tasks",
	}

	cmd.AddCommand(
		newTasksCreateCommand(deps, out),
		newTasksListCommand(deps, out),
		newTasksGetCommand(deps, out),
	)

	return cmd
}

func newTasksCreateCommand(deps Dependencies, out io.Writer) *cobra.Command {
	var agentID string

	cmd := &cobra.Command{
		Use:   "create <prompt>",
		Short: "Create and run a task",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskService, cleanup, err := ensureTaskService(deps)
			if err != nil {
				return err
			}
			defer cleanup()

			task, err := taskService.Create(cmd.Context(), strings.Join(args, " "), agentID)
			if err != nil {
				return err
			}
			return writeJSON(out, task)
		},
	}

	cmd.Flags().StringVar(&agentID, "agent", "", "Optional target agent ID")
	return cmd
}

func newTasksListCommand(deps Dependencies, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List local tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			taskService, cleanup, err := ensureTaskService(deps)
			if err != nil {
				return err
			}
			defer cleanup()

			tasks, err := taskService.List(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(out, tasks)
		},
	}
}

func newTasksGetCommand(deps Dependencies, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one local task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskService, cleanup, err := ensureTaskService(deps)
			if err != nil {
				return err
			}
			defer cleanup()

			task, err := taskService.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(out, task)
		},
	}
}
