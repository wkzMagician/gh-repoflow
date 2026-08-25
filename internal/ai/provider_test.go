package ai

import "testing"

func TestDecodeAnalysisRejectsUnknownFields(t *testing.T) {
	_, err := DecodeAnalysis("{\"type\":\"bug\",\"summary\":\"x\",\"confidence\":0.9,\"action\":\"auto_fix\",\"reason\":\"x\",\"unexpected\":true}")
	if err == nil {
		t.Fatal("expected strict schema error")
	}
}

func TestDecodeAnalysis(t *testing.T) {
	result, err := DecodeAnalysis("{\"type\":\"feature\",\"summary\":\"Add export\",\"confidence\":0.91,\"action\":\"analysis_only\",\"reason\":\"Feature work is human-owned\",\"suspected_files\":[],\"duplicate_candidates\":[]}")
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "feature" || result.Action != "analysis_only" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
