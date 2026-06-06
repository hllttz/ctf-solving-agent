package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMetaAndBuildIncludeSolves(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "metadata.yml")
	if err := os.WriteFile(metaPath, []byte("name: Baby\ncategory: web\nvalue: 500\nsolves: 17\n"), 0644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	meta, err := LoadMeta(metaPath)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.Solves != 17 {
		t.Fatalf("solves = %d", meta.Solves)
	}

	got := Build(meta, filepath.Join(dir, "distfiles"), filepath.Join(dir, "workspace"))
	for _, want := range []string{
		"Points: 500",
		"Solves: 17",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}
