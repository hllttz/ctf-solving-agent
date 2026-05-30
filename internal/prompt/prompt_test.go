package prompt

import "strings"
import "testing"

func TestBuildRemoteServiceRequiresFirstConnection(t *testing.T) {
	meta := &Meta{
		Name:        "remote",
		Category:    "pwn",
		Description: "connect and solve",
		Host:        "127.0.0.1",
		Port:        31337,
	}

	got := Build(meta, "/challenge/distfiles", "/workspace")

	for _, want := range []string{
		"FIRST ACTION REQUIRED",
		"nc host.docker.internal 31337",
		"1. Connect to the service now.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}
