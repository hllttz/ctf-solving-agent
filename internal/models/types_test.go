package models

import "testing"

func TestInspectSpecWithEffort(t *testing.T) {
	got := InspectSpec("claude-sdk/claude-opus-4-6/max")

	if got.Provider != "claude-sdk" {
		t.Fatalf("provider = %q", got.Provider)
	}
	if got.ModelID != "claude-opus-4-6" {
		t.Fatalf("model id = %q", got.ModelID)
	}
	if got.Effort != "max" {
		t.Fatalf("effort = %q", got.Effort)
	}
	if got.ContextWindow != 1_000_000 {
		t.Fatalf("context window = %d", got.ContextWindow)
	}
	if !got.SupportsVision {
		t.Fatalf("supports vision = false")
	}
}

func TestInspectSpecDefaults(t *testing.T) {
	got := InspectSpec("deepseek/deepseek-chat")

	if got.Provider != "deepseek" {
		t.Fatalf("provider = %q", got.Provider)
	}
	if got.ModelID != "deepseek-chat" {
		t.Fatalf("model id = %q", got.ModelID)
	}
	if got.ContextWindow != 64_000 {
		t.Fatalf("context window = %d", got.ContextWindow)
	}
	if got.SupportsVision {
		t.Fatalf("supports vision = true")
	}
}

func TestInspectSpecDeepSeekV4Flash(t *testing.T) {
	got := InspectSpec("deepseek/deepseek-v4-flash")

	if got.Provider != "deepseek" {
		t.Fatalf("provider = %q", got.Provider)
	}
	if got.ModelID != "deepseek-v4-flash" {
		t.Fatalf("model id = %q", got.ModelID)
	}
	if got.ContextWindow != 1_000_000 {
		t.Fatalf("context window = %d", got.ContextWindow)
	}
	if got.SupportsVision {
		t.Fatalf("supports vision = true")
	}
}
