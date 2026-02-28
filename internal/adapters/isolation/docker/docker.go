package docker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type DockerIsolation struct {
	Image       string
	MemoryLimit string
	CPUQuota    string
	WorkDir     string
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
	_, _, _, err := d.Execute(ctx, "sh", []string{"-c", fmt.Sprintf("mkdir -p $(dirname %s) && cat > %s", path, path)}, nil)
	return err
}

func (d *DockerIsolation) Cleanup(_ context.Context) error { return nil }

func (d *DockerIsolation) baseArgs() []string {
	args := []string{"run", "--rm", "--network=none"}
	if d.MemoryLimit != "" {
		args = append(args, "--memory="+d.MemoryLimit)
	}
	if d.WorkDir != "" {
		args = append(args, "-v", d.WorkDir+":/workspace", "-w", "/workspace")
	}
	return args
}
