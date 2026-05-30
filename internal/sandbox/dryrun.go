package sandbox

import (
	"context"
	"fmt"
)

// DryRunSandbox simulates a sandbox without Docker for testing.
type DryRunSandbox struct {
	workspace string
	distfiles string
}

func NewDryRun(workspace, distfiles string) *DryRunSandbox {
	return &DryRunSandbox{workspace: workspace, distfiles: distfiles}
}

func (s *DryRunSandbox) Exec(_ context.Context, command string) (string, error) {
	return fmt.Sprintf("[dry-run] would execute: %s", command), nil
}

func (s *DryRunSandbox) ExecWithTimeout(_ context.Context, command string, timeoutSeconds int) (*ExecResult, error) {
	return &ExecResult{
		ExitCode: 0,
		Stdout:   fmt.Sprintf("[dry-run] would execute with timeout %ds: %s", timeoutSeconds, command),
	}, nil
}

func (s *DryRunSandbox) ReadFile(path string) (string, error) {
	return fmt.Sprintf("[dry-run] would read: %s", path), nil
}

func (s *DryRunSandbox) ReadFileBytes(path string) ([]byte, error) {
	return []byte(fmt.Sprintf("[dry-run] binary content of: %s", path)), nil
}

func (s *DryRunSandbox) WriteFile(path, content string) error {
	return nil
}

func (s *DryRunSandbox) ListFiles(dir string) (string, error) {
	return fmt.Sprintf("[dry-run] would list: %s", dir), nil
}

func (s *DryRunSandbox) Stop(_ context.Context) error   { return nil }
func (s *DryRunSandbox) Remove(_ context.Context) error { return nil }

func (s *DryRunSandbox) Workspace() string   { return s.workspace }
func (s *DryRunSandbox) Distfiles() string   { return s.distfiles }
func (s *DryRunSandbox) ContainerID() string { return "dry-run" }
