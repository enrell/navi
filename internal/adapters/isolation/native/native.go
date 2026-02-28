package native

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type NativeIsolation struct {
	AllowedPaths []string
}

func New(allowedPaths []string) *NativeIsolation {
	return &NativeIsolation{AllowedPaths: allowedPaths}
}

func (n *NativeIsolation) Execute(ctx context.Context, cmd string, args []string, env map[string]string) (int, string, string, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	if len(env) > 0 {
		c.Env = os.Environ()
		for k, v := range env {
			c.Env = append(c.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return -1, "", stderr.String(), err
		}
	}
	return exitCode, stdout.String(), stderr.String(), nil
}

func (n *NativeIsolation) ReadFile(_ context.Context, path string) (string, error) {
	if err := n.checkPath(path); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (n *NativeIsolation) WriteFile(_ context.Context, path, content string) error {
	if err := n.checkPath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func (n *NativeIsolation) Cleanup(_ context.Context) error { return nil }

func (n *NativeIsolation) checkPath(path string) error {
	if len(n.AllowedPaths) == 0 {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for _, allowed := range n.AllowedPaths {
		allowedAbs, _ := filepath.Abs(allowed)
		rel, err := filepath.Rel(allowedAbs, abs)
		if err == nil && len(rel) > 0 && rel[0] != '.' {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside allowed directories", path)
}
