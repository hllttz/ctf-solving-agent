package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadDir reads markdown skills from dir and returns prompt-ready content.
func LoadDir(dir string) (string, error) {
	return loadSelected(dir, nil)
}

// LoadForCategory reads common.md and the markdown skill matching category.
func LoadForCategory(dir, category string) (string, error) {
	selected := map[string]bool{"common.md": true}
	category = strings.ToLower(strings.TrimSpace(category))
	if category != "" {
		selected[category+".md"] = true
	}
	return loadSelected(dir, selected)
}

func loadSelected(dir string, selected map[string]bool) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read skills dir: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.EqualFold(name, "README.md") {
			continue
		}
		if strings.HasSuffix(strings.ToLower(name), ".md") {
			if selected != nil && !selected[strings.ToLower(name)] {
				continue
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read skill %s: %w", name, err)
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("## Additional Solver Skills\n")
			b.WriteString("Use these local skills when they match the challenge. They are optional guidance, not proof of a flag.\n\n")
		}
		b.WriteString("### ")
		b.WriteString(strings.TrimSuffix(name, filepath.Ext(name)))
		b.WriteString("\n")
		b.WriteString(content)
		b.WriteString("\n\n")
	}

	return strings.TrimSpace(b.String()), nil
}
