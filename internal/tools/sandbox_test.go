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
	writtenPath  string
	writtenData  string
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

func (s *toolSandbox) WriteFile(path, content string) error {
	s.writtenPath = path
	s.writtenData = content
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

func TestReadFileRelativePathDoesNotEscapeChallengeDirs(t *testing.T) {
	sb := &toolSandbox{
		files: map[string]string{
			"/challenge/metadata.yml": "metadata",
		},
	}
	tool := NewReadFileTool(sb)

	got, err := tool.InvokableRun(context.Background(), `{"path":"../metadata.yml"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	wantAttempts := []string{"../metadata.yml"}
	if !reflect.DeepEqual(sb.readAttempts, wantAttempts) {
		t.Fatalf("read attempts = %#v, want %#v", sb.readAttempts, wantAttempts)
	}
	if got == "metadata" {
		t.Fatal("escaped relative path read metadata")
	}
}

func TestWriteFileRelativePathWritesUnderWorkspace(t *testing.T) {
	sb := &toolSandbox{}
	tool := NewWriteFileTool(sb)

	got, err := tool.InvokableRun(context.Background(), `{"path":"scripts/solve.py","content":"print(1)\n"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if sb.writtenPath != "/workspace/scripts/solve.py" {
		t.Fatalf("written path = %q, want /workspace/scripts/solve.py", sb.writtenPath)
	}
	if sb.writtenData != "print(1)\n" {
		t.Fatalf("written content = %q", sb.writtenData)
	}
	if got != "File written successfully: /workspace/scripts/solve.py" {
		t.Fatalf("result = %q", got)
	}
}

func TestWriteFileAbsolutePathUsesExactPath(t *testing.T) {
	sb := &toolSandbox{}
	tool := NewWriteFileTool(sb)

	_, err := tool.InvokableRun(context.Background(), `{"path":"/tmp/solve.py","content":"x"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if sb.writtenPath != "/tmp/solve.py" {
		t.Fatalf("written path = %q, want /tmp/solve.py", sb.writtenPath)
	}
}

func TestWriteFileRelativePathDoesNotEscapeWorkspace(t *testing.T) {
	sb := &toolSandbox{}
	tool := NewWriteFileTool(sb)

	_, err := tool.InvokableRun(context.Background(), `{"path":"../metadata.yml","content":"x"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if sb.writtenPath != "../metadata.yml" {
		t.Fatalf("written path = %q, want ../metadata.yml", sb.writtenPath)
	}
}
