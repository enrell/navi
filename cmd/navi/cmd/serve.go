package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newServeCommand(out io.Writer) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the REST API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(phase-1): wire the HTTP adapter and start listening.
			fmt.Fprintf(out, "navi server starting on :%d (not yet implemented)\n", port)
			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to listen on")
	return cmd
}
