// Package docker provides an IsolationPort backed by Docker containers.
// Each agent task runs in an isolated container with controlled mounts.
package docker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DockerIsolation implements ports.IsolationPort using `docker run`.
type DockerIsolation struct {
	Image       string
	MemoryLimit string // e.g. "512m"
	CPUQuota    string // e.g. "0.5"
	WorkDir     string // host path to mount as /workspace
}

func New(image, workDir string) *DockerIsolation {
	return &DockerIsolation{
		Image:       image,
		MemoryLimit: "512m",
		WorkDir:     workDir,
	}
}

func (d *DockerIsolation) Execute(ctx context.Context, cmd string, args []string, env map[string]string) (int, string, string, error) {
	dockerArgs := d.baseArgs()
	// Pass env vars
	for k, v := range env {
		dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	dockerArgs = append(dockerArgs, d.Image, cmd)
	dockerArgs = append(dockerArgs, args...)

	c := exec.CommandContext(ctx, "docker", dockerArgs...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return -1, "", stderr.String(), fmt.Errorf("docker exec: %w", err)
		}
	}
	return exitCode, stdout.String(), stderr.String(), nil
}

func (d *DockerIsolation) ReadFile(ctx context.Context, path string) (string, error) {
	_, stdout, _, err := d.Execute(ctx, "cat", []string{path}, nil)
	if err != nil {
		return "", err
	}
	return stdout, nil
}

func (d *DockerIsolation) WriteFile(ctx context.Context, path, content string) error {
	// Use docker cp: write to a temp container
	// TODO: implement via `docker run --rm -i <image> tee <path>` with stdin
	_, _, _, err := d.Execute(ctx, "sh", []string{"-c", fmt.Sprintf("mkdir -p $(dirname %s) && cat > %s", path, path)}, nil)
	return err
}

func (d *DockerIsolation) Cleanup(ctx context.Context) error {
	// Containers are ephemeral (--rm), nothing to clean.
	return nil
}

func (d *DockerIsolation) baseArgs() []string {
	args := []string{"run", "--rm", "--network=none"}
	if d.MemoryLimit != "" {
		args = append(args, "--memory="+d.MemoryLimit)
	}
	if d.WorkDir != "" {
		args = append(args, "-v", d.WorkDir+":/workspace", "-w", "/workspace")
	}
	_ = strings.Join(args, " ") // satisfy strings import
	return args
}
