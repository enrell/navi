package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	httpserver "navi/internal/adapters/http"
)

func newServeCommand(deps Dependencies, out io.Writer) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the REST API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := fmt.Sprintf(":%d", port)
			fmt.Fprintf(out, "navi API server listening on %s\n", addr)

			srv := httpserver.New(deps.Tasks, deps.Agents)
			return srv.Start(addr)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to listen on")
	return cmd
}
