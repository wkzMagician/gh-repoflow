package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type AnalysisResult struct {
	Type                string   `json:"type"`
	Summary             string   `json:"summary"`
	Confidence          float64  `json:"confidence"`
	Action              string   `json:"action"`
	Reason              string   `json:"reason"`
	SuspectedFiles      []string `json:"suspected_files"`
	DuplicateCandidates []string `json:"duplicate_candidates"`
}

func DecodeAnalysis(content string) (AnalysisResult, error) {
	var result AnalysisResult
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return AnalysisResult{}, fmt.Errorf("analysis result must be strict JSON: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return AnalysisResult{}, fmt.Errorf("analysis result contains trailing data")
	}
	if err := ValidateAnalysis(result); err != nil {
		return AnalysisResult{}, err
	}
	return result, nil
}

func ValidateAnalysis(result AnalysisResult) error {
	switch result.Type {
	case "bug", "feature", "question", "docs", "task", "unknown":
	default:
		return fmt.Errorf("analysis type %q is invalid", result.Type)
	}
	switch result.Action {
	case "analysis_only", "needs_human", "auto_fix":
	default:
		return fmt.Errorf("analysis action %q is invalid", result.Action)
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		return fmt.Errorf("analysis confidence must be between 0 and 1")
	}
	if strings.TrimSpace(result.Summary) == "" || strings.TrimSpace(result.Reason) == "" {
		return fmt.Errorf("analysis summary and reason are required")
	}
	if result.SuspectedFiles == nil || result.DuplicateCandidates == nil {
		return fmt.Errorf("analysis list fields are required and must be arrays")
	}
	return nil
}
