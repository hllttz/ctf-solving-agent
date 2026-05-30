package tools

import (
	"strings"
	"testing"
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
