package challenge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateManualCopiesFilesAndWritesMetadata(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "chall.zip")
	if err := os.WriteFile(src, []byte("zip bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := CreateManual(ManualOptions{
		Root:     filepath.Join(root, "challenges"),
		Name:     "Baby Pwn!",
		Category: "pwn",
		Target:   "nc host 31337",
		Files:    []string{src},
	})
	if err != nil {
		t.Fatalf("CreateManual error: %v", err)
	}
	if got, want := filepath.Base(out.Dir), "baby-pwn"; got != want {
		t.Fatalf("dir basename = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(out.Dir, "distfiles", "chall.zip")); err != nil {
		t.Fatalf("copied file missing: %v", err)
	}

	meta, err := os.ReadFile(filepath.Join(out.Dir, "metadata.yml"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	for _, want := range []string{
		"name: Baby Pwn!",
		"category: pwn",
		"connection_info: nc host 31337",
		"- chall.zip",
	} {
		if !strings.Contains(string(meta), want) {
			t.Fatalf("metadata missing %q:\n%s", want, string(meta))
		}
	}
}

func TestCreateManualInfersNameFromFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "crypto-task.tar.gz")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := CreateManual(ManualOptions{
		Root:  filepath.Join(root, "challenges"),
		Files: []string{src},
	})
	if err != nil {
		t.Fatalf("CreateManual error: %v", err)
	}
	if out.Name != "crypto-task.tar" {
		t.Fatalf("name = %q", out.Name)
	}
}
