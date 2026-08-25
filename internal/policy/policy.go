package policy

import (
	"path/filepath"
	"strings"

	"github.com/repoflow/gh-repoflow/internal/ai"
)

type Config struct {
	AutoFixBugs   bool
	MinConfidence float64
}

type Decision string

const (
	AnalysisOnly Decision = "analysis_only"
	NeedsHuman   Decision = "needs_human"
	AutoFix      Decision = "auto_fix"
)

func Evaluate(result ai.AnalysisResult, cfg Config) Decision {
	if result.Type != "bug" {
		return AnalysisOnly
	}
	if result.Confidence < cfg.MinConfidence || result.Action != string(AutoFix) || !cfg.AutoFixBugs {
		return NeedsHuman
	}
	if len(result.SuspectedFiles) == 0 {
		return NeedsHuman
	}
	for _, file := range result.SuspectedFiles {
		if ForbiddenPath(file) {
			return NeedsHuman
		}
	}
	return AutoFix
}

func ForbiddenPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	lower := strings.ToLower(clean)
	if clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(path) {
		return true
	}
	for _, part := range strings.Split(lower, "/") {
		if part == ".git" || part == ".github" {
			return true
		}
	}
	for _, fragment := range []string{"secret", "credential", "token", "password", "ruleset", "workflow"} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}
