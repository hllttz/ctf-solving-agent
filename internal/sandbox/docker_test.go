package sandbox

import (
	"strings"
	"testing"
)

func TestBinaryFileHintSuggestsUsefulCommands(t *testing.T) {
	got := BinaryFileHint("/challenge/distfiles/blob.bin", 123)

	for _, want := range []string{"Binary file (123 bytes)", "file '/challenge/distfiles/blob.bin'", "xxd", "strings", "binwalk"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}
