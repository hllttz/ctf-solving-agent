package tools

import (
	"context"
	"errors"
	"reflect"
	"testing"

	sandboxpkg "github.com/verialabs/ctf-agent/internal/sandbox"
)

type toolSandbox struct {
	files        map[string]string
	listed       string
	readAttempts []string
}

func (s *toolSandbox) Exec(context.Context, string) (string, error) {
	return "", nil
}

func (s *toolSandbox) ExecWithTimeout(context.Context, string, int) (*sandboxpkg.ExecResult, error) {
	return nil, nil
}

func (s *toolSandbox) ReadFile(path string) (string, error) {
	s.readAttempts = append(s.readAttempts, path)
	if s.files == nil {
		return "", errors.New("not found")
	}
	content, ok := s.files[path]
	if !ok {
		return "", errors.New("not found")
	}
	return content, nil
}

func (s *toolSandbox) ReadFileBytes(string) ([]byte, error) {
	return nil, nil
}

func (s *toolSandbox) WriteFile(string, string) error {
	return nil
}

func (s *toolSandbox) ListFiles(dir string) (string, error) {
	s.listed = dir
	return "listed " + dir, nil
}

func (s *toolSandbox) Stop(context.Context) error {
	return nil
}

func (s *toolSandbox) Remove(context.Context) error {
	return nil
}

func (s *toolSandbox) Workspace() string {
	return "/workspace"
}

func (s *toolSandbox) Distfiles() string {
	return "/challenge/distfiles"
}

func (s *toolSandbox) ContainerID() string {
	return "test"
}

func TestListFilesDefaultsToDistfiles(t *testing.T) {
	sb := &toolSandbox{}
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
	sb := &toolSandbox{}
	tool := NewListFilesTool(sb)

	_, err := tool.InvokableRun(context.Background(), `{"path":"/workspace"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if sb.listed != "/workspace" {
		t.Fatalf("listed %q, want /workspace", sb.listed)
	}
}

func TestReadFileRelativePathPrefersDistfiles(t *testing.T) {
	sb := &toolSandbox{
		files: map[string]string{
			"/challenge/distfiles/readme.txt": "distfile content",
			"/workspace/readme.txt":           "workspace content",
		},
	}
	tool := NewReadFileTool(sb)

	got, err := tool.InvokableRun(context.Background(), `{"path":"readme.txt"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if got != "distfile content" {
		t.Fatalf("result = %q", got)
	}
	wantAttempts := []string{"/challenge/distfiles/readme.txt"}
	if !reflect.DeepEqual(sb.readAttempts, wantAttempts) {
		t.Fatalf("read attempts = %#v, want %#v", sb.readAttempts, wantAttempts)
	}
}

func TestReadFileRelativePathFallsBackToWorkspace(t *testing.T) {
	sb := &toolSandbox{
		files: map[string]string{
			"/workspace/notes.txt": "workspace content",
		},
	}
	tool := NewReadFileTool(sb)

	got, err := tool.InvokableRun(context.Background(), `{"path":"notes.txt"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if got != "workspace content" {
		t.Fatalf("result = %q", got)
	}
	wantAttempts := []string{"/challenge/distfiles/notes.txt", "/workspace/notes.txt"}
	if !reflect.DeepEqual(sb.readAttempts, wantAttempts) {
		t.Fatalf("read attempts = %#v, want %#v", sb.readAttempts, wantAttempts)
	}
}

func TestReadFileAbsolutePathUsesExactPath(t *testing.T) {
	sb := &toolSandbox{
		files: map[string]string{
			"/tmp/out.txt": "tmp content",
		},
	}
	tool := NewReadFileTool(sb)

	got, err := tool.InvokableRun(context.Background(), `{"path":"/tmp/out.txt"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if got != "tmp content" {
		t.Fatalf("result = %q", got)
	}
	wantAttempts := []string{"/tmp/out.txt"}
	if !reflect.DeepEqual(sb.readAttempts, wantAttempts) {
		t.Fatalf("read attempts = %#v, want %#v", sb.readAttempts, wantAttempts)
	}
}
