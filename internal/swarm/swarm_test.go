package swarm

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBumpQueuesTargetedOperatorGuidance(t *testing.T) {
	sw := New("baby", t.TempDir(), []string{"openai/test"}, nil, "ctf-sandbox")
	sw.solvers["openai/test"] = nil

	if !sw.Bump("openai/test", "try the signed cookie path") {
		t.Fatalf("Bump returned false")
	}

	insights, ok := sw.takePendingBump("openai/test")
	if !ok {
		t.Fatalf("pending bump was not queued")
	}
	if insights != "try the signed cookie path" {
		t.Fatalf("pending bump = %q", insights)
	}
	if _, ok := sw.takePendingBump("openai/test"); ok {
		t.Fatalf("pending bump was not consumed")
	}

	items, _ := sw.Bus().CheckFor("openai/test", 0)
	if len(items) != 1 {
		t.Fatalf("target solver visible items = %d: %#v", len(items), items)
	}
	if items[0].Target != "openai/test" || !strings.Contains(items[0].Content, "signed cookie") {
		t.Fatalf("targeted bus item = %#v", items[0])
	}

	items, _ = sw.Bus().CheckFor("anthropic/test", 0)
	if len(items) != 0 {
		t.Fatalf("other solver saw targeted item: %#v", items)
	}
}

func TestBumpRejectsUnknownModel(t *testing.T) {
	sw := New("baby", t.TempDir(), []string{"openai/test"}, nil, "ctf-sandbox")

	if sw.Bump("openai/test", "try harder") {
		t.Fatalf("Bump returned true before solver was registered")
	}
}

func TestBumpWakesPendingDelay(t *testing.T) {
	sw := New("baby", t.TempDir(), []string{"openai/test"}, nil, "ctf-sandbox")
	sw.solvers["openai/test"] = nil

	done := make(chan bool, 1)
	go func() {
		done <- sw.waitForBumpOrDelay(context.Background(), "openai/test", time.Hour)
	}()

	if !sw.Bump("openai/test", "stop waiting") {
		t.Fatalf("Bump returned false")
	}

	select {
	case ok := <-done:
		if !ok {
			t.Fatalf("wait returned false")
		}
	case <-time.After(time.Second):
		t.Fatalf("bump did not wake pending delay")
	}
}
