package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"navi/internal/adapters/storage/sqlite"
	agentsvc "navi/internal/core/services/agent"
	tasksvc "navi/internal/core/services/task"
)

func ensureTaskService(deps Dependencies) (*tasksvc.Service, func(), error) {
	if deps.Tasks != nil {
		return deps.Tasks, func() {}, nil
	}
	if deps.Chat == nil {
		return nil, nil, fmt.Errorf("tasks: chat service is not wired")
	}

	dbPath, err := taskDBPath()
	if err != nil {
		return nil, nil, fmt.Errorf("tasks: task db path: %w", err)
	}

	repo, err := sqlite.NewTaskRepository(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("tasks: init sqlite task repo: %w", err)
	}

	cleanup := func() {
		_ = repo.Close()
	}
	return tasksvc.New(repo, deps.Chat), cleanup, nil
}

func ensureAgentService(ctx context.Context, deps Dependencies) (*agentsvc.Service, *localAgentSyncer, func(), error) {
	if deps.Agents != nil {
		return deps.Agents, nil, func() {}, nil
	}

	dbPath, err := agentDBPath()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("agents: agent db path: %w", err)
	}

	repo, err := sqlite.NewAgentRepository(dbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("agents: init sqlite agent repo: %w", err)
	}

	cleanup := func() {
		_ = repo.Close()
	}

	roots, err := defaultAgentRoots()
	if err != nil {
		cleanup()
		return nil, nil, nil, fmt.Errorf("agents: resolve agent roots: %w", err)
	}

	if _, err := bootstrapAgents(ctx, repo, roots); err != nil {
		cleanup()
		return nil, nil, nil, fmt.Errorf("agents: bootstrap agents: %w", err)
	}

	return agentsvc.New(repo), &localAgentSyncer{repo: repo, roots: roots}, cleanup, nil
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
