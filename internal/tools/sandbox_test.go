package tools

import (
	"context"
	"testing"

	sandboxpkg "github.com/verialabs/ctf-agent/internal/sandbox"
)

type listFilesSandbox struct {
	listed string
}

func (s *listFilesSandbox) Exec(context.Context, string) (string, error) {
	return "", nil
}

func (s *listFilesSandbox) ExecWithTimeout(context.Context, string, int) (*sandboxpkg.ExecResult, error) {
	return nil, nil
}

func (s *listFilesSandbox) ReadFile(string) (string, error) {
	return "", nil
}

func (s *listFilesSandbox) ReadFileBytes(string) ([]byte, error) {
	return nil, nil
}

func (s *listFilesSandbox) WriteFile(string, string) error {
	return nil
}

func (s *listFilesSandbox) ListFiles(dir string) (string, error) {
	s.listed = dir
	return "listed " + dir, nil
}

func (s *listFilesSandbox) Stop(context.Context) error {
	return nil
}

func (s *listFilesSandbox) Remove(context.Context) error {
	return nil
}

func (s *listFilesSandbox) Workspace() string {
	return "/workspace"
}

func (s *listFilesSandbox) Distfiles() string {
	return "/challenge/distfiles"
}

func (s *listFilesSandbox) ContainerID() string {
	return "test"
}

func TestListFilesDefaultsToDistfiles(t *testing.T) {
	sb := &listFilesSandbox{}
	tool := NewListFilesTool(sb)

	got, err := tool.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if sb.listed != "/challenge/distfiles" {
		t.Fatalf("listed %q, want /challenge/distfiles", sb.listed)
	}
	if got != "listed /challenge/distfiles" {
		t.Fatalf("result = %q", got)
	}
}

func TestListFilesUsesExplicitPath(t *testing.T) {
	sb := &listFilesSandbox{}
	tool := NewListFilesTool(sb)

	_, err := tool.InvokableRun(context.Background(), `{"path":"/workspace"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if sb.listed != "/workspace" {
		t.Fatalf("listed %q, want /workspace", sb.listed)
	}
}
