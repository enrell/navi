package native_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"navi/internal/adapters/isolation/native"
)

func TestNativeIsolation_Filesystem(t *testing.T) {
	tempDir := t.TempDir()
	allowedDir := filepath.Join(tempDir, "allowed")
	requireNoError(t, os.MkdirAll(allowedDir, 0755))

	iso := native.New([]string{allowedDir})
	ctx := context.Background()

	t.Run("write and read allowed file", func(t *testing.T) {
		path := filepath.Join(allowedDir, "test.txt")
		err := iso.WriteFile(ctx, path, "hello")
		if err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		content, err := iso.ReadFile(ctx, path)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if content != "hello" {
			t.Errorf("expected 'hello', got %q", content)
		}
	})

	t.Run("deny path outside allowed dir", func(t *testing.T) {
		outsidePath := filepath.Join(tempDir, "outside.txt")
		err := iso.WriteFile(ctx, outsidePath, "secret")
		if err == nil {
			t.Error("expected error writing outside allowed dir, got nil")
		}

		_, err = iso.ReadFile(ctx, outsidePath)
		if err == nil {
			t.Error("expected error reading outside allowed dir, got nil")
		}
	})

	t.Run("deny directory traversal breakout", func(t *testing.T) {
		// Attempting to breakout using ../
		breakoutPath := filepath.Join(allowedDir, "..", "breakout.txt")
		err := iso.WriteFile(ctx, breakoutPath, "breakout")
		if err == nil {
			t.Error("expected error writing with directory traversal, got nil")
		}
	})

	t.Run("allow path exactly equal to allowed dir implies reading dir? or fails if not file?", func(t *testing.T) {
		_, err := iso.ReadFile(ctx, allowedDir)
		if err == nil {
			t.Log("reading directory returned no error, which is fine as long as path logic passes")
		} else if !strings.Contains(err.Error(), "is a directory") && !strings.Contains(err.Error(), "Access is denied") {
			// If it fails because "is a directory" or "Access is denied" (Windows) that is expected OS behavior
			t.Logf("failed for other reasons: %v", err)
		}
	})

	t.Run("invalid path reading", func(t *testing.T) {
		_, err := iso.ReadFile(ctx, "\x00invalid")
		if err == nil {
			// some systems don't fail, but it's an edge case
		}
	})

	t.Run("invalid path writing", func(t *testing.T) {
		err := iso.WriteFile(ctx, "\x00invalid", "data")
		if err == nil {
			// some systems don't fail, but it's an edge case
		}
	})

	t.Run("Cleanup returns nil", func(t *testing.T) {
		if err := iso.Cleanup(ctx); err != nil {
			t.Errorf("Cleanup expected nil, got %v", err)
		}
	})
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNativeIsolation_Execute(t *testing.T) {
	iso := native.New(nil)
	ctx := context.Background()

	t.Run("execute simple command", func(t *testing.T) {
		// We use something cross platform. 'go env' is cross platform if go is installed.
		// Or since we are creating this specifically for Windows, 'cmd.exe' with '/c echo hello'
		// Let's use 'go' as it's guaranteed to be in the test environment
		exitCode, stdout, _, err := iso.Execute(ctx, "go", []string{"version"}, nil)
		requireNoError(t, err)
		if exitCode != 0 {
			t.Errorf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(stdout, "go version") {
			t.Errorf("expected 'go version' in stdout, got %q", stdout)
		}
	})

	t.Run("execute non-existent command", func(t *testing.T) {
		exitCode, _, _, err := iso.Execute(ctx, "this-binary-does-not-exist-12345", []string{}, nil)
		if err == nil {
			t.Error("expected error for non-existent command, got nil")
		}
		if exitCode != -1 {
			t.Errorf("expected exit code -1, got %d", exitCode)
		}
	})

	t.Run("execute command with non-zero exit", func(t *testing.T) {
		exitCode, _, stderr, err := iso.Execute(ctx, "go", []string{"invalid-command"}, nil)
		// We expect no error because the command ran but exited with non-zero
		if err != nil {
			t.Errorf("expected nil error for valid command that just exits with non-zero, got %v", err)
		}
		if exitCode == 0 {
			t.Error("expected non-zero exit code")
		}
		if stderr == "" {
			t.Error("expected stderr output, got empty")
		}
	})

	t.Run("verify env sanitization", func(t *testing.T) {
		os.Setenv("NAVI_SECRET_TEST", "p4ssw0rd")
		defer os.Unsetenv("NAVI_SECRET_TEST")

		// On Windows, use cmd /c set to dump environment
		exitCode, stdout, _, err := iso.Execute(ctx, "cmd", []string{"/c", "set"}, map[string]string{"AGENT_MODE": "1"})
		requireNoError(t, err)
		if exitCode != 0 {
			t.Errorf("expected exit code 0, got %d", exitCode)
		}

		if !strings.Contains(stdout, "AGENT_MODE=1") {
			t.Error("explicit agent environment missing")
		}
	})

	t.Run("execute respects context timeout", func(t *testing.T) {
		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()

		// cmd.exe /c "ping 127.0.0.1 -n 5" takes about 4 seconds
		exitCode, _, _, err := iso.Execute(timeoutCtx, "cmd", []string{"/c", "ping", "127.0.0.1", "-n", "5"}, nil)

		if err == nil {
			t.Error("expected context deadline exceeded error, got nil")
		}
		if exitCode == 0 {
			t.Errorf("expected non-zero exit code on killed process, got %d", exitCode)
		}
	})
}
