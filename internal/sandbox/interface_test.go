package sandbox

import (
	"strings"
	"testing"
)

func TestFormatExecResultIncludesStderrAndExitCode(t *testing.T) {
	got := FormatExecResult(&ExecResult{
		ExitCode: 2,
		Stdout:   "out\n",
		Stderr:   "err\n",
	})

	for _, want := range []string{"out", "[stderr]\nerr", "[exit 2]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatExecResultEmpty(t *testing.T) {
	if got := FormatExecResult(&ExecResult{}); got != "(no output)" {
		t.Fatalf("got %q", got)
	}
}
