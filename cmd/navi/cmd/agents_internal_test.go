package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentsSync_UsesWorkspaceConfigAgents(t *testing.T) {
	tmp := t.TempDir()
	configRoot := filepath.Join(tmp, "configs", "agents", "coder")
	if err := os.MkdirAll(configRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "config.toml"), []byte("type = \"generic\"\nname = \"Coder\"\ncapabilities = [\"filesystem:workspace:rw\"]\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "AGENT.md"), []byte("You are coder"), 0o600); err != nil {
		t.Fatalf("write agent md: %v", err)
	}

	oldAgentDBPath := agentDBPath
	agentDBPath = func() (string, error) { return filepath.Join(tmp, "agents.db"), nil }
	defer func() { agentDBPath = oldAgentDBPath }()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	var buf bytes.Buffer
	root := NewRootCommand(Dependencies{}, &buf)
	root.SetArgs([]string{"agents", "sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"synced": 1`) {
		t.Fatalf("output %q should contain synced count", out)
	}
	if !strings.Contains(out, `"message": "agents synced"`) {
		t.Fatalf("output %q should contain success message", out)
	}
	buf.Reset()

	root = NewRootCommand(Dependencies{}, &buf)
	root.SetArgs([]string{"agents", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute list: %v", err)
	}
	if !strings.Contains(buf.String(), `"id": "coder"`) {
		t.Fatalf("list output %q should contain synced agent", buf.String())
	}
}
