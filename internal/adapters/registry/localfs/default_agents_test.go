package localfs_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"navi/internal/adapters/registry/localfs"
)

func TestRepositoryDefaultAgents_LoadAndValidate(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	root := filepath.Join(repoRoot, "configs", "agents")

	agents, err := localfs.LoadGenericAgentsFromRoots([]string{root})
	if err != nil {
		t.Fatalf("LoadGenericAgentsFromRoots error: %v", err)
	}

	if len(agents) < 5 {
		t.Fatalf("loaded %d agents, want at least 5 defaults", len(agents))
	}

	want := map[string]bool{
		"orchestrator": false,
		"planner":      false,
		"researcher":   false,
		"coder":        false,
		"tester":       false,
	}

	for _, a := range agents {
		id := a.ID()
		if _, ok := want[id]; ok {
			want[id] = true
		}
		if a.Type() != "generic" {
			t.Fatalf("agent %q type = %q, want generic", id, a.Type())
		}
		if a.SystemPrompt() == "" {
			t.Fatalf("agent %q has empty system prompt", id)
		}
	}

	for id, loaded := range want {
		if !loaded {
			t.Fatalf("default agent %q was not loaded", id)
		}
	}
}
