package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	orchestratorsvc "navi/internal/core/services/orchestrator"

	"github.com/spf13/cobra"
)

func newReplCommand(deps Dependencies, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "repl",
		Short: "Start an interactive chat REPL",
		Long:  "Starts a simple terminal chat session so you can quickly test the configured agent/model.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deps.Orchestrator == nil && deps.Chat == nil {
				return fmt.Errorf("repl: neither orchestrator nor chat service is wired")
			}

			fmt.Fprintln(out, "Navi REPL — type 'exit' or 'quit' to leave")
			scanner := bufio.NewScanner(cmd.InOrStdin())

			for {
				fmt.Fprint(out, "> ")
				if !scanner.Scan() {
					if err := scanner.Err(); err != nil {
						return fmt.Errorf("repl: read input: %w", err)
					}
					fmt.Fprintln(out)
					return nil
				}

				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}

				switch strings.ToLower(line) {
				case "exit", "quit", ":q":
					fmt.Fprintln(out, "Bye.")
					return nil
				}

				var (
					reply string
					err   error
				)

				fmt.Fprintf(out, "\nUser: %s\n", line)
				if deps.Orchestrator != nil {
					var trace []orchestratorsvc.TraceEvent
					reply, trace, err = deps.Orchestrator.AskWithTrace(cmd.Context(), line)
					if err == nil {
						for _, event := range trace {
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
					reply, err = deps.Chat.Chat(cmd.Context(), line)
				}
				if err != nil {
					fmt.Fprintf(out, "error: %v\n", err)
					continue
				}

				fmt.Fprintf(out, "Orchestrator: %s\n", reply)
			}
		},
	}
}
