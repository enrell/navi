package localfs_test

import (
	"os"
	"path/filepath"
	"testing"

	"navi/internal/adapters/registry/localfs"
	"navi/internal/core/domain"
)

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoadAgentsFromRoots_Empty(t *testing.T) {
	agents, err := localfs.LoadAgentsFromRoots([]string{filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("len = %d, want 0", len(agents))
	}
}

func TestLoadAgentsFromRoots_LoadsConfigAndMarkdown(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agents")
	writeFile(t, filepath.Join(root, "coder", "config.toml"), "type = \"generic\"\nname = \"Coder\"\ncapabilities = [\"filesystem:workspace:rw\", \"exec:go\"]\n")
	writeFile(t, filepath.Join(root, "coder", "AGENT.md"), "You are coder\nMore details")

	agents, err := localfs.LoadAgentsFromRoots([]string{root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("len = %d, want 1", len(agents))
	}
	if agents[0].ID != "coder" {
		t.Errorf("ID = %q, want coder", agents[0].ID)
	}
	if agents[0].Name != "Coder" {
		t.Errorf("Name = %q, want Coder", agents[0].Name)
	}
	if agents[0].Description != "You are coder" {
		t.Errorf("Description = %q, want first line from AGENT.md", agents[0].Description)
	}
	if agents[0].Type != "generic" {
		t.Errorf("Type = %q, want generic", agents[0].Type)
	}
}

func TestLoadAgentsFromRoots_IDAndStatusOverride(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agents")
	writeFile(t, filepath.Join(root, "foo", "config.toml"), "id = \"orchestrator\"\nname = \"Orchestrator\"\nstatus = \"modified\"\n")

	agents, err := localfs.LoadAgentsFromRoots([]string{root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("len = %d, want 1", len(agents))
	}
	if agents[0].ID != "orchestrator" {
		t.Errorf("ID = %q, want orchestrator", agents[0].ID)
	}
	if agents[0].Status != domain.AgentStatusModified {
		t.Errorf("Status = %q, want modified", agents[0].Status)
	}
}

func TestLoadAgentsFromRoots_DuplicateID_LastRootWins(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "a")
	rootB := filepath.Join(t.TempDir(), "b")
	writeFile(t, filepath.Join(rootA, "coder", "config.toml"), "name = \"Coder A\"\n")
	writeFile(t, filepath.Join(rootB, "coder", "config.toml"), "name = \"Coder B\"\n")

	agents, err := localfs.LoadAgentsFromRoots([]string{rootA, rootB})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("len = %d, want 1", len(agents))
	}
	if agents[0].Name != "Coder B" {
		t.Errorf("Name = %q, want Coder B", agents[0].Name)
	}
}

func TestLoadAgentsFromRoots_InvalidTOML(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agents")
	writeFile(t, filepath.Join(root, "bad", "config.toml"), "this is [ not toml")

	_, err := localfs.LoadAgentsFromRoots([]string{root})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadGenericAgentsFromRoots_CustomPromptFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agents")
	writeFile(t, filepath.Join(root, "researcher", "config.toml"), "type = \"generic\"\nprompt = \"SYSTEM.md\"\nname = \"Researcher\"\n")
	writeFile(t, filepath.Join(root, "researcher", "SYSTEM.md"), "You are researcher\nUse citations")

	agents, err := localfs.LoadGenericAgentsFromRoots([]string{root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("len = %d, want 1", len(agents))
	}
	if agents[0].Config().PromptFile != "SYSTEM.md" {
		t.Errorf("PromptFile = %q, want SYSTEM.md", agents[0].Config().PromptFile)
	}
	if agents[0].SystemPrompt() != "You are researcher\nUse citations" {
		t.Errorf("unexpected system prompt: %q", agents[0].SystemPrompt())
	}
}

func TestLoadGenericAgentsFromRoots_ExplicitMissingPromptFile_Error(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agents")
	writeFile(t, filepath.Join(root, "broken", "config.toml"), "type = \"generic\"\nprompt = \"MISSING.md\"\n")

	_, err := localfs.LoadGenericAgentsFromRoots([]string{root})
	if err == nil {
		t.Fatal("expected missing prompt file error")
	}
}
