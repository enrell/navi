package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"navi/internal/adapters/isolation/git"
)

// A mock isolation port that just records calls and implements simple file writing
type MockIsolation struct {
	writes map[string]string
	err    error
}

func (m *MockIsolation) Execute(ctx context.Context, cmd string, args []string, env map[string]string) (int, string, string, error) {
	return 0, "", "", m.err
}

func (m *MockIsolation) ReadFile(ctx context.Context, path string) (string, error) {
	return m.writes[path], m.err
}

func (m *MockIsolation) WriteFile(ctx context.Context, path, content string) error {
	if m.err != nil {
		return m.err
	}
	m.writes[path] = content

	// mock file creation on disk for git to detect
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	return os.WriteFile(path, []byte(content), 0644)
}

func (m *MockIsolation) Cleanup(ctx context.Context) error {
	return m.err
}

func TestGitAuditIsolation_FileWritesTriggerCommits(t *testing.T) {
	tempDir := t.TempDir()

	inner := &MockIsolation{writes: make(map[string]string)}
	// We wrap the inner isolation port with our Git adapter
	iso := git.NewGitAudit(inner, tempDir, "Agent-007", "Task-123")

	ctx := context.Background()
	testPath := filepath.Join(tempDir, "hello.txt")
	err := iso.WriteFile(ctx, testPath, "world")

	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if inner.writes[testPath] != "world" {
		t.Error("inner WriteFile was not actually called or content mismatch")
	}

	// Verify a .git repo was created
	gitDir := filepath.Join(tempDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Fatalf("expected .git directory to be initialized, but it wasn't")
	}

	// Verify git status is clean (everything committed)
	cmd := exec.CommandContext(ctx, "git", "-C", tempDir, "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v", string(out))
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("expected clean working tree, but got: %s", string(out))
	}

	// Verify commit log
	cmd = exec.CommandContext(ctx, "git", "-C", tempDir, "log", "-1", "--oneline")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v", string(out))
	}
	if !strings.Contains(string(out), "agent: Agent-007 action: Task-123") {
		t.Errorf("commit message pattern mismatch: %s", string(out))
	}
}

func TestGitAuditIsolation_ProxyMethods(t *testing.T) {
	inner := &MockIsolation{writes: make(map[string]string)}
	iso := git.NewGitAudit(inner, "workspace", "agent", "task")
	ctx := context.Background()

	// Revert the error mapping to test proxy
	inner.writes["foo"] = "bar"
	val, _ := iso.ReadFile(ctx, "foo")
	if val != "bar" {
		t.Error("ReadFile proxy failed")
	}

	exitCode, _, _, _ := iso.Execute(ctx, "cmd", nil, nil)
	if exitCode != 0 {
		t.Error("Execute proxy failed")
	}

	err := iso.Cleanup(ctx)
	if err != nil {
		t.Error("Cleanup proxy failed")
	}
}

func TestGitAuditIsolation_WriteFileErrors(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("inner write error propagates", func(t *testing.T) {
		inner := &MockIsolation{err: os.ErrPermission}
		iso := git.NewGitAudit(inner, tempDir, "a", "t")

		err := iso.WriteFile(context.Background(), "path", "cont")
		if err != os.ErrPermission {
			t.Error("Inner WriteFile error was not propagated")
		}
	})

	t.Run("git init fails on invalid dir", func(t *testing.T) {
		// Mock a failure by clearing the PATH so `git` is not found
		oldPath := os.Getenv("PATH")
		os.Setenv("PATH", "")
		defer os.Setenv("PATH", oldPath)

		inner := &MockIsolation{writes: make(map[string]string)}
		iso := git.NewGitAudit(inner, tempDir, "a", "t")

		err := iso.WriteFile(context.Background(), filepath.Join(tempDir, "f.txt"), "cont")
		if err == nil {
			t.Error("expected git execution error due to missing git binary, got nil")
		}
	})

	t.Run("git add fails when git directory exists but is corrupted", func(t *testing.T) {
		tempDir2 := t.TempDir()
		// Make a fake .git file so os.Stat(".git") succeeds, skipping init, but
		// `git add` fails because it's not a real repo
		if err := os.WriteFile(filepath.Join(tempDir2, ".git"), []byte("corrupt"), 0644); err != nil {
			t.Fatal(err)
		}

		inner := &MockIsolation{writes: make(map[string]string)}
		iso := git.NewGitAudit(inner, tempDir2, "a", "t")

		err := iso.WriteFile(context.Background(), filepath.Join(tempDir2, "f2.txt"), "cont")
		if err == nil {
			t.Error("expected git execution error due to corrupt git repo, got nil")
		}
	})
}
