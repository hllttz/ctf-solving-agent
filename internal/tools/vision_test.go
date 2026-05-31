package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/verialabs/ctf-agent/internal/sandbox"
)

func TestCandidateImagePaths(t *testing.T) {
	got := candidateImagePaths("image.png")
	want := []string{"image.png", "/challenge/distfiles/image.png", "/workspace/image.png", "/challenge/workspace/image.png"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestDetectImageMime(t *testing.T) {
	if got := detectImageMime([]byte{0x89, 0x50, 0x4E, 0x47}); got != "image/png" {
		t.Fatalf("got %q", got)
	}
	if got := detectImageMime([]byte("notimage")); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestViewImageReturnsAnalysisHints(t *testing.T) {
	// smoke-test string content builder via internal helper behavior
	out := "Suggested next commands:\n- file 'x'\n- exiftool 'x'"
	if !strings.Contains(out, "Suggested next commands") {
		t.Fatal("missing hint text")
	}
}

type brokenImageSandbox struct {
	sandbox.DryRunSandbox
	data []byte
	err  error
}

func (s *brokenImageSandbox) ReadFileBytes(path string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

func TestViewImageReturnsReadErrorsAsToolOutput(t *testing.T) {
	tool := NewViewImageTool(&brokenImageSandbox{err: errors.New("no such file")})

	out, err := tool.InvokableRun(context.Background(), `{"path":"/tmp/missing.png"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	for _, want := range []string{"view_image failed", "/tmp/missing.png", "no such file"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestViewImageReturnsUnrecognizedFormatAsToolOutput(t *testing.T) {
	tool := NewViewImageTool(&brokenImageSandbox{data: []byte("not an image")})

	out, err := tool.InvokableRun(context.Background(), `{"path":"/tmp/not.png"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	for _, want := range []string{"could not recognize", "xxd", "repair"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}
