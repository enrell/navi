package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	orchestratorsvc "navi/internal/core/services/orchestrator"
	"navi/internal/telemetry"

	"github.com/spf13/cobra"
)

func newReplCommand(deps Dependencies, out io.Writer) *cobra.Command {
	var plain bool

	cmd := &cobra.Command{
		Use:   "repl",
		Short: "Start an interactive chat REPL",
		Long:  "Starts a simple terminal chat session so you can quickly test the configured agent/model.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInteractiveSession(cmd, deps, out, plain, true)
		},
	}

	cmd.Flags().BoolVar(&plain, "plain", false, "Force the line-based REPL instead of the full-screen TUI")
	return cmd
}

func runPlainRepl(cmd *cobra.Command, deps Dependencies, out io.Writer) error {
	if deps.Orchestrator == nil && deps.Chat == nil {
		telemetry.Logger().Error("repl_not_wired")
		return fmt.Errorf("repl: neither orchestrator nor chat service is wired")
	}
	telemetry.Logger().Info("repl_start")

	fmt.Fprintln(out, "Navi REPL — type ':help' for commands, 'exit' or 'quit' to leave")
	scanner := bufio.NewScanner(cmd.InOrStdin())
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	for {
		fmt.Fprint(out, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				telemetry.Logger().Error("repl_read_error", "error", err.Error())
				return fmt.Errorf("repl: read input: %w", err)
			}
			telemetry.Logger().Info("repl_eof")
			fmt.Fprintln(out)
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch strings.ToLower(line) {
		case "exit", "quit", ":q":
			telemetry.Logger().Info("repl_exit_command")
			fmt.Fprintln(out, "Bye.")
			return nil
		case ":help":
			printReplHelp(out)
			continue
		case ":agents":
			if err := printReplAgents(cmd.Context(), deps, out); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
			}
			continue
		}

		var (
			reply string
			err   error
		)
		turnCtx, traceID := telemetry.EnsureTraceID(cmd.Context())
		telemetry.Logger().Info("repl_user_input", "trace_id", traceID, "chars", len(line))

		fmt.Fprintf(out, "\nUser: %s\n", line)
		if deps.Orchestrator != nil {
			var trace []orchestratorsvc.TraceEvent
			reply, trace, err = deps.Orchestrator.AskWithTrace(turnCtx, line)
			if err == nil {
				for _, event := range trace {
					telemetry.Logger().Info("repl_trace_event", "trace_id", traceID, "type", string(event.Type), "tool", event.Tool, "chars", len(event.Content))
					switch event.Type {
					case "thinking":
						if strings.TrimSpace(event.Content) != "" {
							fmt.Fprintf(out, "Thinking: %s\n", event.Content)
						}
					case "tool_response":
						fmt.Fprintf(out, "Tool Response [%s]: %s\n", event.Tool, event.Content)
					}
				}
			}
		} else {
			reply, err = deps.Chat.Chat(turnCtx, line)
		}
		if err != nil {
			telemetry.Logger().Error("repl_turn_error", "trace_id", traceID, "error", err.Error())
			fmt.Fprintf(out, "error: %v\n", err)
			continue
		}
		telemetry.Logger().Info("repl_turn_completed", "trace_id", traceID, "reply_chars", len(reply))

		fmt.Fprintf(out, "Orchestrator: %s\n", reply)
	}

	return nil
}

func printReplHelp(out io.Writer) {
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  :help    Show REPL commands")
	fmt.Fprintln(out, "  :agents  List registered agents")
	fmt.Fprintln(out, "  :q       Exit the REPL")
	fmt.Fprintln(out, "  exit     Exit the REPL")
	fmt.Fprintln(out, "  quit     Exit the REPL")
}

func printReplAgents(ctx context.Context, deps Dependencies, out io.Writer) error {
	agentService, _, cleanup, err := ensureAgentService(ctx, deps)
	if err != nil {
		return err
	}
	defer cleanup()

	agents, err := agentService.List(ctx)
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		fmt.Fprintln(out, "No agents registered.")
		return nil
	}

	fmt.Fprintln(out, "Registered agents:")
	for _, agent := range agents {
		fmt.Fprintf(out, "  - %s: %s\n", agent.ID, agent.Name)
	}
	return nil
}
