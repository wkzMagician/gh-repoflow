package template

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:default
var defaults embed.FS

var required = []string{
	".github/workflows/ci.yml",
	".github/workflows/ai-issue-analysis.yml",
	".github/workflows/ai-bug-fix.yml",
	".github/workflows/ai-debug.yml",
	".github/workflows/release.yml",
	".github/pull_request_template.md",
	".github/ISSUE_TEMPLATE/bug.yml",
	".github/ISSUE_TEMPLATE/feature.yml",
	"CODEOWNERS",
	"CONTRIBUTING.md",
}

func RequiredFiles() []string {
	return append([]string(nil), required...)
}

// Ensure writes only missing files. Existing project-specific workflows and
// documentation are preserved; repoflow.yml is managed separately by config.
func Ensure(root string) ([]string, error) {
	created := make([]string, 0)
	err := fs.WalkDir(defaults, "default", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(path, "default"), "/")
		target := filepath.Join(root, filepath.FromSlash(relative))
		if _, err := os.Stat(target); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		data, err := fs.ReadFile(defaults, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		created = append(created, relative)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("generate repository templates: %w", err)
	}
	return created, nil
}
