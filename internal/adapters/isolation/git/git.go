package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"navi/internal/core/domain"
)

// GitAuditIsolation wraps an existing IsolationPort and automatically
// provisions and commits to a git repository in the workspace.
type GitAuditIsolation struct {
	inner     domain.IsolationPort
	workspace string
	agentID   string
	taskID    string
}

func NewGitAudit(inner domain.IsolationPort, workspace, agentID, taskID string) *GitAuditIsolation {
	return &GitAuditIsolation{
		inner:     inner,
		workspace: workspace,
		agentID:   agentID,
		taskID:    taskID,
	}
}

func (g *GitAuditIsolation) ReadFile(ctx context.Context, path string) (string, error) {
	return g.inner.ReadFile(ctx, path)
}

func (g *GitAuditIsolation) Execute(ctx context.Context, cmd string, args []string, env map[string]string) (int, string, string, error) {
	return g.inner.Execute(ctx, cmd, args, env)
}

func (g *GitAuditIsolation) Cleanup(ctx context.Context) error {
	return g.inner.Cleanup(ctx)
}

// WriteFile intercepts the write operation and runs git add/commit afterwards.
func (g *GitAuditIsolation) WriteFile(ctx context.Context, path, content string) error {
	// 1. Delegate actual write logic to the protected adapter
	if err := g.inner.WriteFile(ctx, path, content); err != nil {
		return err
	}

	// 2. Ensure git exists in the workspace
	gitDir := filepath.Join(g.workspace, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := g.runGit(ctx, "init"); err != nil {
			return fmt.Errorf("git init failed: %w", err)
		}
	}

	// 3. Auto-commit the change
	msg := fmt.Sprintf("agent: %s action: %s", g.agentID, g.taskID)
	if err := g.runGit(ctx, "add", "."); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Set author manually to prevent machines without global git config from dying
	if err := g.runGit(ctx, "-c", "user.name=Navi System", "-c", "user.email=system@navi.local", "commit", "-m", msg); err != nil {
		// If there is nothing to commit, don't fail the operation
		// A git commit fails if there are no changes, so ignore that particular state.
	}

	return nil
}

func (g *GitAuditIsolation) runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.workspace
	return cmd.Run()
}
