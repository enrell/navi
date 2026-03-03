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
	"navi/internal/telemetry"
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
	traceID := telemetry.TraceID(ctx)
	telemetry.Logger().Info("agent_sync_start", "trace_id", traceID, "roots", len(s.roots))
	agents, err := localfs.LoadAgentsFromRoots(s.roots)
	if err != nil {
		telemetry.Logger().Error("agent_sync_load_failed", "trace_id", traceID, "error", err.Error())
		return 0, err
	}
	if err := s.repo.Seed(ctx, agents); err != nil {
		telemetry.Logger().Error("agent_sync_seed_failed", "trace_id", traceID, "error", err.Error())
		return 0, err
	}
	telemetry.Logger().Info("agent_sync_done", "trace_id", traceID, "synced", len(agents))
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

func bootstrapAgents(ctx context.Context, repo *sqlite.AgentRepository, roots []string) (int, error) {
	traceID := telemetry.TraceID(ctx)
	telemetry.Logger().Info("agent_bootstrap_start", "trace_id", traceID, "roots", len(roots))
	agents, err := localfs.LoadAgentsFromRoots(roots)
	if err != nil {
		telemetry.Logger().Error("agent_bootstrap_load_failed", "trace_id", traceID, "error", err.Error())
		return 0, err
	}
	if err := repo.Seed(ctx, agents); err != nil {
		telemetry.Logger().Error("agent_bootstrap_seed_failed", "trace_id", traceID, "error", err.Error())
		return 0, err
	}
	telemetry.Logger().Info("agent_bootstrap_done", "trace_id", traceID, "count", len(agents))
	return len(agents), nil
}

func newServeCommand(deps Dependencies, out io.Writer) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the REST API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, traceID := telemetry.EnsureTraceID(cmd.Context())
			telemetry.Logger().Info("serve_start", "trace_id", traceID, "port", port)
			if deps.Chat == nil {
				telemetry.Logger().Error("serve_chat_not_wired", "trace_id", traceID)
				return fmt.Errorf("serve: chat service is not wired")
			}

			taskService := deps.Tasks
			if taskService == nil {
				dbPath, err := taskDBPath()
				if err != nil {
					telemetry.Logger().Error("serve_task_db_path_failed", "trace_id", traceID, "error", err.Error())
					return fmt.Errorf("serve: task db path: %w", err)
				}

				repo, err := sqlite.NewTaskRepository(dbPath)
				if err != nil {
					telemetry.Logger().Error("serve_task_repo_init_failed", "trace_id", traceID, "error", err.Error(), "db_path", dbPath)
					return fmt.Errorf("serve: init sqlite task repo: %w", err)
				}
				telemetry.Logger().Info("serve_task_repo_ready", "trace_id", traceID, "db_path", dbPath)
				taskService = tasksvc.New(repo, deps.Chat)
			}

			agentService := deps.Agents
			var agentRepo *sqlite.AgentRepository
			if agentService == nil {
				dbPath, err := agentDBPath()
				if err != nil {
					telemetry.Logger().Error("serve_agent_db_path_failed", "trace_id", traceID, "error", err.Error())
					return fmt.Errorf("serve: agent db path: %w", err)
				}

				repo, err := sqlite.NewAgentRepository(dbPath)
				if err != nil {
					telemetry.Logger().Error("serve_agent_repo_init_failed", "trace_id", traceID, "error", err.Error(), "db_path", dbPath)
					return fmt.Errorf("serve: init sqlite agent repo: %w", err)
				}
				telemetry.Logger().Info("serve_agent_repo_ready", "trace_id", traceID, "db_path", dbPath)
				agentRepo = repo
				agentService = agentsvc.New(repo)
			}

			addr := fmt.Sprintf(":%d", port)
			fmt.Fprintf(out, "navi API server listening on %s\n", addr)

			srv := httpserver.New(taskService, agentService)

			if agentRepo != nil {
				roots, err := defaultAgentRoots()
				if err != nil {
					telemetry.Logger().Error("serve_agent_roots_failed", "trace_id", traceID, "error", err.Error())
					return fmt.Errorf("serve: resolve agent roots: %w", err)
				}

				n, err := bootstrapAgents(ctx, agentRepo, roots)
				if err != nil {
					telemetry.Logger().Error("serve_agent_bootstrap_failed", "trace_id", traceID, "error", err.Error())
					return fmt.Errorf("serve: bootstrap agents: %w", err)
				}
				if n > 0 {
					fmt.Fprintf(out, "loaded %d agent(s) from data files\n", n)
				}

				srv.SetAgentSyncer(&localAgentSyncer{repo: agentRepo, roots: roots})
			}

			err := srv.Start(addr)
			if err != nil {
				telemetry.Logger().Error("serve_stopped_with_error", "trace_id", traceID, "error", err.Error())
				return err
			}
			telemetry.Logger().Info("serve_stopped", "trace_id", traceID)
			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to listen on")
	return cmd
}
