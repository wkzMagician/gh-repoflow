package policy

import (
	"testing"

	"github.com/repoflow/gh-repoflow/internal/ai"
)

func TestFeatureIsAnalysisOnly(t *testing.T) {
	result := ai.AnalysisResult{Type: "feature", Confidence: 0.99, Action: "auto_fix"}
	if got := Evaluate(result, Config{AutoFixBugs: true, MinConfidence: 0.85}); got != AnalysisOnly {
		t.Fatalf("got %s", got)
	}
}

func TestBugNeedsHumanBelowThreshold(t *testing.T) {
	result := ai.AnalysisResult{Type: "bug", Confidence: 0.84, Action: "auto_fix"}
	if got := Evaluate(result, Config{AutoFixBugs: true, MinConfidence: 0.85}); got != NeedsHuman {
		t.Fatalf("got %s", got)
	}
}

func TestBugCannotTouchGovernance(t *testing.T) {
	result := ai.AnalysisResult{
		Type: "bug", Confidence: 0.99, Action: "auto_fix",
		SuspectedFiles: []string{".github/workflows/ci.yml"},
	}
	if got := Evaluate(result, Config{AutoFixBugs: true, MinConfidence: 0.85}); got != NeedsHuman {
		t.Fatalf("got %s", got)
	}
}
