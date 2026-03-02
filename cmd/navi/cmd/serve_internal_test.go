package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"navi/internal/adapters/storage/sqlite"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestBootstrapAgents_LoadsFromConfigTomlAndAgentMarkdown(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agents")
	writeFile(t, filepath.Join(root, "coder", "config.toml"), "type = \"generic\"\nname = \"Coder\"\ncapabilities = [\"filesystem:workspace:rw\"]\n")
	writeFile(t, filepath.Join(root, "coder", "AGENT.md"), "You are coder")

	repo, err := sqlite.NewAgentRepository(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewAgentRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	n, err := bootstrapAgents(context.Background(), repo, []string{root})
	if err != nil {
		t.Fatalf("bootstrapAgents error: %v", err)
	}
	if n != 1 {
		t.Fatalf("synced = %d, want 1", n)
	}

	agent, err := repo.FindByID(context.Background(), "coder")
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if agent.Type != "generic" {
		t.Errorf("Type = %q, want generic", agent.Type)
	}
	if agent.Name != "Coder" {
		t.Errorf("Name = %q, want Coder", agent.Name)
	}
}

func TestBootstrapAgents_EmptyRoots_NoError(t *testing.T) {
	repo, err := sqlite.NewAgentRepository(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewAgentRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	n, err := bootstrapAgents(context.Background(), repo, []string{filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("synced = %d, want 0", n)
	}
}
