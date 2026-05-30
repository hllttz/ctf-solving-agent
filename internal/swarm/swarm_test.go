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

func TestRunBroadcastsStrategyHintBeforeSolvers(t *testing.T) {
	sw := NewWithStrategy("chal", "/tmp/chal", nil, nil, "image", "16g", "prioritize service")

	result := sw.Run(t.Context(), "prompt")
	if result.Status == 0 {
		t.Fatalf("unexpected running result")
	}

	items := sw.bus.All()
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Author != "coordinator" || items[0].Content != "prioritize service" {
		t.Fatalf("unexpected broadcast: %#v", items[0])
	}
}
