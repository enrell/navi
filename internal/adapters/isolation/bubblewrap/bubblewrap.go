// Package bubblewrap provides an IsolationPort backed by bwrap (Bubblewrap).
// Bubblewrap is a lightweight sandboxing tool available on Linux (Arch, Fedora, etc.)
// that uses user namespaces — no daemon required.
package bubblewrap

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// BubblewrapIsolation implements ports.IsolationPort using the `bwrap` binary.
type BubblewrapIsolation struct {
	// WorkDir is bind-mounted read-write into the sandbox as /workspace.
	WorkDir string
	// ROBinds are additional (host path, sandbox path) read-only bind mounts.
	ROBinds [][2]string
}

func New(workDir string) *BubblewrapIsolation {
	return &BubblewrapIsolation{WorkDir: workDir}
}

func (b *BubblewrapIsolation) Execute(ctx context.Context, cmd string, args []string, env map[string]string) (int, string, string, error) {
	bwrapArgs := b.baseArgs()
	// Inject environment variables via --setenv
	for k, v := range env {
		bwrapArgs = append(bwrapArgs, "--setenv", k, v)
	}
	bwrapArgs = append(bwrapArgs, "--", cmd)
	bwrapArgs = append(bwrapArgs, args...)

	c := exec.CommandContext(ctx, "bwrap", bwrapArgs...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return -1, "", stderr.String(), fmt.Errorf("bwrap exec: %w", err)
		}
	}
	return exitCode, stdout.String(), stderr.String(), nil
}

func (b *BubblewrapIsolation) ReadFile(ctx context.Context, path string) (string, error) {
	_, stdout, _, err := b.Execute(ctx, "cat", []string{path}, nil)
	return stdout, err
}

func (b *BubblewrapIsolation) WriteFile(ctx context.Context, path, content string) error {
	_, _, _, err := b.Execute(ctx, "sh", []string{"-c", fmt.Sprintf("mkdir -p $(dirname %q) && cat > %q", path, path)}, nil)
	return err
}

func (b *BubblewrapIsolation) Cleanup(_ context.Context) error { return nil }

func (b *BubblewrapIsolation) baseArgs() []string {
	args := []string{
		"--unshare-all", // unshare all namespaces
		"--new-session", // new session so can't get a controlling tty
		"--die-with-parent",
		// Minimal filesystem
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/lib", "/lib",
		"--ro-bind", "/lib64", "/lib64",
		"--ro-bind", "/bin", "/bin",
		"--ro-bind", "/sbin", "/sbin",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmp-overlay", "/tmp",
	}
	if b.WorkDir != "" {
		args = append(args, "--bind", b.WorkDir, "/workspace")
		args = append(args, "--chdir", "/workspace")
	}
	for _, bind := range b.ROBinds {
		args = append(args, "--ro-bind", bind[0], bind[1])
	}
	return args
}
