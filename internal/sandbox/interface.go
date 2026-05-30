package sandbox

import (
	"context"
	"fmt"
	"strings"
)

// ExecResult is the structured result of a sandbox command.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// FormatExecResult converts a structured command result into model-facing text.
func FormatExecResult(result *ExecResult) string {
	if result == nil {
		return "(no output)"
	}
	var parts []string
	if strings.TrimSpace(result.Stdout) != "" {
		parts = append(parts, result.Stdout)
	}
	if strings.TrimSpace(result.Stderr) != "" {
		parts = append(parts, "[stderr]\n"+result.Stderr)
	}
	if result.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("[exit %d]", result.ExitCode))
	}
	out := strings.TrimSpace(strings.Join(parts, "\n"))
	if out == "" {
		out = "(no output)"
	}
	const maxLen = 24000
	if len(out) > maxLen {
		return out[:maxLen] + "\n... [output truncated]"
	}
	return out
}

// Sandbox is the common interface for sandbox implementations.
type Sandbox interface {
	Exec(ctx context.Context, command string) (string, error)
	ExecWithTimeout(ctx context.Context, command string, timeoutSeconds int) (*ExecResult, error)
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
