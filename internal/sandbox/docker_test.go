package sandbox

import (
	"strings"
	"testing"
)

func TestBinaryFileHintSuggestsUsefulCommands(t *testing.T) {
	got := BinaryFileHint("/challenge/distfiles/blob.bin", 123)

	for _, want := range []string{"Binary file (123 bytes)", "file '/challenge/distfiles/blob.bin'", "xxd", "strings", "binwalk"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestDockerRunArgsIncludeLifecycleOptions(t *testing.T) {
	args := dockerRunArgs(DockerOptions{
		Image:       "ctf-sandbox",
		Name:        "ctf-test",
		MemoryLimit: "4g",
	}, "/host/workspace", "/host/distfiles", "/host/metadata.yml", true)
	got := strings.Join(args, " ")

	for _, want := range []string{
		"--label ctf-agent=true",
		"--memory 4g",
		"/host/metadata.yml:/challenge/metadata.yml:ro",
		"--add-host host.docker.internal:host-gateway",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in args:\n%s", want, got)
		}
	}
}

func TestDockerRunArgsDefaultMemory(t *testing.T) {
	args := dockerRunArgs(DockerOptions{
		Image: "ctf-sandbox",
		Name:  "ctf-test",
	}, "/host/workspace", "/host/distfiles", "/host/metadata.yml", false)
	got := strings.Join(args, " ")

	if !strings.Contains(got, "--memory 16g") {
		t.Fatalf("expected default memory in args:\n%s", got)
	}
	if strings.Contains(got, "metadata.yml:/challenge/metadata.yml") {
		t.Fatalf("metadata mounted unexpectedly:\n%s", got)
	}
}
