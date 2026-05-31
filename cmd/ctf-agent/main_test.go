package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/verialabs/ctf-agent/internal/config"
	"github.com/verialabs/ctf-agent/internal/solver"
)

func TestValidateModelSpecsMissingKeys(t *testing.T) {
	err := validateModelSpecs([]string{
		"openai/gpt-5.4",
		"deepseek/deepseek-v4-flash",
	}, map[string]string{
		"openai": "",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	for _, want := range []string{"OPENAI_API_KEY", "DEEPSEEK_API_KEY"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func TestValidateModelSpecsAcceptsAliases(t *testing.T) {
	err := validateModelSpecs([]string{
		"claude-sdk/claude-opus-4-6",
		"google/gemini-3-flash-preview",
		"codex/gpt-5.4",
	}, map[string]string{
		"anthropic": "sk-ant",
		"gemini":    "sk-gemini",
		"openai":    "sk-openai",
	})
	if err != nil {
		t.Fatalf("validateModelSpecs error: %v", err)
	}
}

func TestStatusText(t *testing.T) {
	cases := []struct {
		status solver.Status
		want   string
	}{
		{solver.FlagFound, "solved"},
		{solver.GaveUp, "gave up"},
		{solver.Error, "error"},
		{solver.Cancelled, "cancelled"},
	}
	for _, tc := range cases {
		got := statusText(&solver.Result{Status: tc.status})
		if got != tc.want {
			t.Fatalf("statusText(%d) = %q", tc.status, got)
		}
	}
}

func TestRunRejectsMissingTargetAndFileWithoutPrompting(t *testing.T) {
	cmd := runCmd(&config.Config{})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	cmd.SetOut(&stderr)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "provide at least --target or --file") {
		t.Fatalf("error = %q", got)
	}
	if strings.Contains(stderr.String(), "靶机地址") {
		t.Fatalf("unexpected interactive prompt: %q", stderr.String())
	}
}
