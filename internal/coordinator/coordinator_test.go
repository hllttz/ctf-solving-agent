package coordinator

import (
	"strings"
	"testing"

	"github.com/verialabs/ctf-agent/internal/prompt"
)

func TestInitialStrategyForRemoteWeb(t *testing.T) {
	got := initialStrategy(&prompt.Meta{
		Category:    "web",
		Host:        "localhost",
		Port:        8080,
		ServiceType: "web",
	})

	for _, want := range []string{
		"prioritize the live service",
		"enumerate HTTP surface",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestInitialStrategyForForensics(t *testing.T) {
	got := initialStrategy(&prompt.Meta{Category: "forensics"})
	if !strings.Contains(got, "preserve originals") {
		t.Fatalf("unexpected strategy:\n%s", got)
	}
}
