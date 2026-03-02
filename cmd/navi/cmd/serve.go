package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	httpserver "navi/internal/adapters/http"
	"navi/internal/adapters/registry/localfs"
	"navi/internal/adapters/storage/sqlite"
	"navi/internal/config"
	agentsvc "navi/internal/core/services/agent"
	tasksvc "navi/internal/core/services/task"
)

var taskDBPath = defaultTaskDBPath
var agentDBPath = defaultAgentDBPath

func defaultTaskDBPath() (string, error) {
	baseDir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "tasks.db"), nil
}

func defaultAgentDBPath() (string, error) {
	baseDir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "agents.db"), nil
}

type localAgentSyncer struct {
	repo  *sqlite.AgentRepository
	roots []string
}

func (s *localAgentSyncer) Sync(ctx context.Context) (int, error) {
	agents, err := localfs.LoadAgentsFromRoots(s.roots)
	if err != nil {
		return 0, err
	}
	if err := s.repo.Seed(ctx, agents); err != nil {
		return 0, err
	}
	return len(agents), nil
}

func defaultAgentRoots() ([]string, error) {
	baseDir, err := config.Dir()
	if err != nil {
		return nil, err
	}

	roots := []string{filepath.Join(baseDir, "agents")}

	wd, err := os.Getwd()
	if err == nil {
		roots = append(roots, filepath.Join(wd, "configs", "agents"))
	}

	return roots, nil
}

func newServeCommand(deps Dependencies, out io.Writer) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the REST API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deps.Chat == nil {
				return fmt.Errorf("serve: chat service is not wired")
			}

			taskService := deps.Tasks
			if taskService == nil {
				dbPath, err := taskDBPath()
				if err != nil {
					return fmt.Errorf("serve: task db path: %w", err)
				}

				repo, err := sqlite.NewTaskRepository(dbPath)
				if err != nil {
					return fmt.Errorf("serve: init sqlite task repo: %w", err)
				}
				taskService = tasksvc.New(repo, deps.Chat)
			}

			agentService := deps.Agents
			var agentRepo *sqlite.AgentRepository
			if agentService == nil {
				dbPath, err := agentDBPath()
				if err != nil {
					return fmt.Errorf("serve: agent db path: %w", err)
				}

				repo, err := sqlite.NewAgentRepository(dbPath)
				if err != nil {
					return fmt.Errorf("serve: init sqlite agent repo: %w", err)
				}
				agentRepo = repo
				agentService = agentsvc.New(repo)
			}

			addr := fmt.Sprintf(":%d", port)
			fmt.Fprintf(out, "navi API server listening on %s\n", addr)

			srv := httpserver.New(taskService, agentService)

			if agentRepo != nil {
				roots, err := defaultAgentRoots()
				if err != nil {
					return fmt.Errorf("serve: resolve agent roots: %w", err)
				}
				srv.SetAgentSyncer(&localAgentSyncer{repo: agentRepo, roots: roots})
			}

			return srv.Start(addr)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to listen on")
	return cmd
}
