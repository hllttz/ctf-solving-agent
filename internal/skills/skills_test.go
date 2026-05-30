package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDirFormatsMarkdownSkills(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "crypto.md"), []byte("try sage\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("nope"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs only"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"## Additional Solver Skills",
		"### crypto",
		"try sage",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "nope") {
		t.Fatalf("loaded non-markdown file:\n%s", got)
	}
	if strings.Contains(got, "docs only") {
		t.Fatalf("loaded README as a skill:\n%s", got)
	}
}

func TestLoadDirMissingDirectoryIsEmpty(t *testing.T) {
	got, err := LoadDir(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q", got)
	}
}
