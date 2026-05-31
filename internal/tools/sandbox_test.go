package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/verialabs/ctf-agent/internal/sandbox"
)

type failingReadSandbox struct {
	sandbox.DryRunSandbox
}

func (s failingReadSandbox) ReadFile(path string) (string, error) {
	return "", errors.New("cat /missing: no such file or directory")
}

func TestReadFileToolReturnsSandboxErrorsAsToolOutput(t *testing.T) {
	tool := NewReadFileTool(&failingReadSandbox{})

	out, err := tool.InvokableRun(context.Background(), `{"path":"/missing"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	for _, want := range []string{"read_file failed", "/missing", "no such file"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}
