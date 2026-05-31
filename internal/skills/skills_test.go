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

func TestLoadForCategoryLoadsCommonAndMatchingCategory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"common.md": "general checks\n",
		"rev.md":    "reverse checks\n",
		"crypto.md": "crypto checks\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := LoadForCategory(dir, "REV")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"### common", "general checks", "### rev", "reverse checks"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "crypto checks") {
		t.Fatalf("loaded unrelated skill:\n%s", got)
	}
}

func TestLoadForCategoryWithoutMatchLoadsCommonOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "common.md"), []byte("general checks\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web.md"), []byte("web checks\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadForCategory(dir, "pwn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "general checks") {
		t.Fatalf("missing common skill:\n%s", got)
	}
	if strings.Contains(got, "web checks") {
		t.Fatalf("loaded unrelated skill:\n%s", got)
	}
}
