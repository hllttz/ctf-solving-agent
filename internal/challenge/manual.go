package challenge

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type ManualOptions struct {
	Root        string
	Name        string
	Category    string
	Target      string
	Description string
	Files       []string
}

type ManualResult struct {
	Dir   string
	Name  string
	Files []string
}

func CreateManual(opts ManualOptions) (*ManualResult, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = inferName(opts)
	}
	if name == "" {
		name = "manual-challenge"
	}
	slug := slugify(name)
	if slug == "" {
		slug = "manual-challenge"
	}

	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "challenges"
	}
	challengeDir := filepath.Join(root, slug)
	distDir := filepath.Join(challengeDir, "distfiles")
	if err := os.MkdirAll(distDir, 0755); err != nil {
		return nil, fmt.Errorf("create distfiles dir: %w", err)
	}

	copied := make([]string, 0, len(opts.Files))
	for _, src := range opts.Files {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		name, err := copyFileToDir(src, distDir)
		if err != nil {
			return nil, err
		}
		copied = append(copied, name)
	}

	meta := map[string]any{
		"name":            name,
		"category":        defaultString(opts.Category, "misc"),
		"description":     defaultString(opts.Description, "Manual challenge created from local inputs."),
		"connection_info": strings.TrimSpace(opts.Target),
		"files":           copied,
	}
	data, err := yaml.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(challengeDir, "metadata.yml"), data, 0644); err != nil {
		return nil, fmt.Errorf("write metadata: %w", err)
	}

	return &ManualResult{Dir: challengeDir, Name: name, Files: copied}, nil
}

func copyFileToDir(src, dstDir string) (string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("stat file %s: %w", src, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("file %s is a directory", src)
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", src, err)
	}

	name := filepath.Base(src)
	dst := filepath.Join(dstDir, name)
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return "", fmt.Errorf("copy file %s: %w", src, err)
	}
	return name, nil
}

func inferName(opts ManualOptions) string {
	for _, f := range opts.Files {
		if strings.TrimSpace(f) != "" {
			base := filepath.Base(f)
			ext := filepath.Ext(base)
			return strings.TrimSuffix(base, ext)
		}
	}
	target := strings.TrimSpace(opts.Target)
	if target != "" {
		return target
	}
	return ""
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	reInvalid := regexp.MustCompile(`[<>:"/\\|?*.\x00-\x1f]+`)
	s = reInvalid.ReplaceAllString(s, "")
	reSpace := regexp.MustCompile(`[\s_]+`)
	s = reSpace.ReplaceAllString(s, "-")
	reOther := regexp.MustCompile(`[^a-z0-9-]+`)
	s = reOther.ReplaceAllString(s, "")
	reDash := regexp.MustCompile(`-+`)
	s = reDash.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
