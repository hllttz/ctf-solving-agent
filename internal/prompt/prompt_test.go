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
		"Each bash call is a fresh process",
		"pwntools script",
		"1. Connect to the service now.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildIncludesImageGuidance(t *testing.T) {
	meta := &Meta{
		Name:     "image",
		Category: "forensics",
		Files:    []string{"chall.png"},
	}

	got := Build(meta, "/challenge/distfiles", "/workspace")

	for _, want := range []string{
		"## Image Guidance",
		"Call view_image first",
		"repair magic bytes",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildIncludesBinaryGuidance(t *testing.T) {
	meta := &Meta{
		Name:     "rev",
		Category: "rev",
		Files:    []string{"chall"},
	}

	got := Build(meta, "/challenge/distfiles", "/workspace")

	for _, want := range []string{
		"## Binary Analysis Tools",
		"pyghidra",
		"checksec",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildWebConnectionGuidance(t *testing.T) {
	meta := &Meta{
		Name:        "web",
		Category:    "web",
		Host:        "localhost",
		Port:        8080,
		ServiceType: "web",
	}

	got := Build(meta, "/challenge/distfiles", "/workspace")

	for _, want := range []string{
		"curl -i http://host.docker.internal:8080/",
		"web_fetch for simple GET/POST",
		"cookies, sessions, redirects",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}
