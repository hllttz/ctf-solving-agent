package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verialabs/ctf-agent/internal/prompt"
	"github.com/verialabs/ctf-agent/internal/swarm"
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

func TestBroadcastSendsMessageToSwarms(t *testing.T) {
	c := NewWithOptions("", nil, nil, "", "", 1, "")
	c.swarms["challenge"] = swarm.NewWithOptions("challenge", "", nil, nil, "", "")

	count := c.Broadcast("try the admin path")
	if count != 1 {
		t.Fatalf("broadcast count = %d", count)
	}

	items := c.swarms["challenge"].Bus().All()
	if len(items) != 1 {
		t.Fatalf("items len = %d", len(items))
	}
	if items[0].Content != "try the admin path" {
		t.Fatalf("content = %q", items[0].Content)
	}
}

func TestBroadcastToTargetsOneChallenge(t *testing.T) {
	c := NewWithOptions("", nil, nil, "", "", 1, "")
	c.swarms["a"] = swarm.NewWithOptions("a", "", nil, nil, "", "")
	c.swarms["b"] = swarm.NewWithOptions("b", "", nil, nil, "", "")

	if got := c.BroadcastTo("a", "hello"); got != 1 {
		t.Fatalf("broadcast count = %d", got)
	}
	if len(c.swarms["a"].Bus().All()) != 1 {
		t.Fatal("challenge a not updated")
	}
	if len(c.swarms["b"].Bus().All()) != 0 {
		t.Fatal("challenge b should not receive")
	}
}

func TestReadRecentTrace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace-challenge-model-001.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"start\"}\n{\"type\":\"finish\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lines, err := ReadRecentTrace(dir, "challenge", "model", 1)
	if err != nil {
		t.Fatalf("ReadRecentTrace error: %v", err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "finish") {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestKillAndBumpMissingChallenge(t *testing.T) {
	c := NewWithOptions("", nil, nil, "", "", 1, "")
	if c.Kill("missing") {
		t.Fatal("expected kill false")
	}
	if c.Bump("missing", "model", "insight") {
		t.Fatal("expected bump false")
	}
}

func TestSpawnRejectsAlreadyRunningChallenge(t *testing.T) {
	c := NewWithOptions("", nil, nil, "", "", 1, "")
	c.swarms["challenge"] = swarm.NewWithOptions("challenge", "", nil, nil, "", "")

	if c.Spawn(context.Background(), "challenge") {
		t.Fatal("expected spawn false")
	}
}
