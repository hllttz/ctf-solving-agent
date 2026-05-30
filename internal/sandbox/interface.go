package sandbox

import "context"

// Sandbox is the common interface for sandbox implementations.
type Sandbox interface {
	Exec(ctx context.Context, command string) (string, error)
	ReadFile(path string) (string, error)
	ReadFileBytes(path string) ([]byte, error)
	WriteFile(path, content string) error
	ListFiles(dir string) (string, error)
	Stop(ctx context.Context) error
	Remove(ctx context.Context) error
	Workspace() string
	Distfiles() string
	ContainerID() string
}
