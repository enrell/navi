package bubblewrap

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type BubblewrapIsolation struct {
	WorkDir string
	ROBinds [][2]string
}

func New(workDir string) *BubblewrapIsolation {
	return &BubblewrapIsolation{WorkDir: workDir}
}

func (b *BubblewrapIsolation) Execute(ctx context.Context, cmd string, args []string, env map[string]string) (int, string, string, error) {
	bwrapArgs := b.baseArgs()
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
		"--unshare-all",
		"--new-session",
		"--die-with-parent",
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
