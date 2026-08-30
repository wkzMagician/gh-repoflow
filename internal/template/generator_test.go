package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWritesAllRequiredFiles(t *testing.T) {
	root := t.TempDir()
	created, err := Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != len(required) {
		t.Fatalf("created %d files, want %d", len(created), len(required))
	}
	for _, relative := range required {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
	created, err = Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 0 {
		t.Fatalf("second Ensure created files: %v", created)
	}
}

func TestAIWorkflowBoundaries(t *testing.T) {
	root := t.TempDir()
	if _, err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	analysis, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(".github/workflows/ai-issue-analysis.yml")))
	if err != nil {
		t.Fatal(err)
	}
	analysisText := string(analysis)
	if !strings.Contains(analysisText, "types: [opened, edited, reopened]") ||
		!strings.Contains(analysisText, "contents: read") ||
		!strings.Contains(analysisText, "issues: write") ||
		!strings.Contains(analysisText, "MAX_ISSUES_PER_USER") ||
		!strings.Contains(analysisText, "ai-rate-limited") ||
		strings.Contains(analysisText, "pull-requests: write") {
		t.Fatal("analysis workflow does not have the intended least-privilege boundary")
	}
	fix, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(".github/workflows/ai-bug-fix.yml")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fix), "workflow_dispatch:") || !strings.Contains(string(fix), "git pr create") && !strings.Contains(string(fix), "gh pr create") {
		t.Fatal("bug-fix workflow is missing its isolated PR path")
	}
}

func TestGeneratedWorkflowsHaveRepoFlowAttribution(t *testing.T) {
	root := t.TempDir()
	if _, err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	header := "# Managed by RepoFlow: https://github.com/wkzMagician/gh-repoflow\n"
	for _, relative := range []string{
		".github/workflows/ai-bug-fix.yml",
		".github/workflows/ai-debug.yml",
		".github/workflows/ai-issue-analysis.yml",
		".github/workflows/ci.yml",
		".github/workflows/release.yml",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(data), header) {
			t.Fatalf("%s is missing RepoFlow attribution", relative)
		}
	}
}
