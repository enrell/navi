package isolation

import (
	"context"
	"navi/pkg/types"
)

type IsolationAdapter interface {
	Type() string
	Setup(ctx context.Context, config types.AgentConfig) error
	Execute(ctx context.Context, cmd string, args []string, env map[string]string) (int, string, string, error)
	CopyFile(ctx context.Context, src, dst string) error
	ReadFile(ctx context.Context, path string) (string, error)
	WriteFile(ctx context.Context, path string, content string) error
	Cleanup(ctx context.Context) error
}

type DockerAdapter interface {
	IsolationAdapter
	CreateContainer(ctx context.Context, config types.AgentConfig) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string) error
	RemoveContainer(ctx context.Context, containerID string) error
}

type BubblewrapAdapter interface {
	IsolationAdapter
	Launch(ctx context.Context, config types.AgentConfig) error
}
