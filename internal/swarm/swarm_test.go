package swarm

import (
	"strings"
	"testing"
)

func TestGatherSiblingInsightsExcludesCurrentModel(t *testing.T) {
	sw := New("chal", "/tmp/chal", nil, nil, "image")
	sw.bus.Post("model-a", "own finding")
	sw.bus.Post("model-b", "sibling finding")

	got := sw.gatherSiblingInsights("model-a")

	if strings.Contains(got, "own finding") {
		t.Fatalf("included own finding: %s", got)
	}
	if !strings.Contains(got, "[model-b]: sibling finding") {
		t.Fatalf("missing sibling finding: %s", got)
	}
}

func TestGatherSiblingInsightsEmpty(t *testing.T) {
	sw := New("chal", "/tmp/chal", nil, nil, "image")

	got := sw.gatherSiblingInsights("model-a")

	if got != "No sibling insights available yet." {
		t.Fatalf("got %q", got)
	}
}
