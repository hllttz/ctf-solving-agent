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

func TestBuildDiscoversDistfilesWhenMetadataOmitsFiles(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "distfiles")
	if err := os.Mkdir(dist, 0755); err != nil {
		t.Fatalf("mkdir distfiles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "photo.png"), []byte("png"), 0644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "solver.elf"), []byte("elf"), 0644); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dist, "nested"), 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got := Build(&Meta{Name: "Baby", Category: "web"}, dist, filepath.Join(dir, "workspace"))
	for _, want := range []string{
		"- photo.png",
		"Hint: Image file - use view_image to inspect",
		"- solver.elf",
		"## Image Guidance",
		"## Binary Analysis Tools",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "nested") {
		t.Fatalf("prompt included directory entry:\n%s", got)
	}
}

func TestBuildDeduplicatesMetadataAndDistfileNames(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "distfiles")
	if err := os.Mkdir(dist, 0755); err != nil {
		t.Fatalf("mkdir distfiles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "chall.zip"), []byte("zip"), 0644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	got := Build(&Meta{
		Name:     "Baby",
		Category: "misc",
		Files:    []string{"/challenge/distfiles/chall.zip", "distfiles/chall.zip"},
	}, dist, filepath.Join(dir, "workspace"))

	if count := strings.Count(got, "- chall.zip"); count != 1 {
		t.Fatalf("chall.zip count = %d:\n%s", count, got)
	}
}
