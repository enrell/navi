package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	orchestratorsvc "navi/internal/core/services/orchestrator"
	"navi/internal/telemetry"

	"github.com/spf13/cobra"
)

func newReplCommand(deps Dependencies, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "repl",
		Short: "Start an interactive chat REPL",
		Long:  "Starts a simple terminal chat session so you can quickly test the configured agent/model.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deps.Orchestrator == nil && deps.Chat == nil {
				telemetry.Logger().Error("repl_not_wired")
				return fmt.Errorf("repl: neither orchestrator nor chat service is wired")
			}
			telemetry.Logger().Info("repl_start")

			fmt.Fprintln(out, "Navi REPL — type 'exit' or 'quit' to leave")
			scanner := bufio.NewScanner(cmd.InOrStdin())

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
		},
	}
}
