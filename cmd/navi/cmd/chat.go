package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func newChatCommand(deps Dependencies, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "chat <message>",
		Short: "Send a single message and print the reply",
		Example: `  navi chat "What is the capital of France?"
  navi chat Tell me a joke`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			message := strings.Join(args, " ")
			reply, err := deps.Chat.Chat(cmd.Context(), message)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, reply)
			return nil
		},
	}
}
