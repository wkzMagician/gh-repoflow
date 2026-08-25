package config

import "testing"

func TestParseDocumentedConfig(t *testing.T) {
	input := "repository:\n" +
		"  visibility: private\n" +
		"branches:\n" +
		"  main:\n" +
		"    approvals: 3\n" +
		"  dev:\n" +
		"    approvals: 1\n" +
		"developers:\n" +
		"  - alice\n" +
		"  - \"bob\"\n" +
		"ci:\n" +
		"  enabled: true\n" +
		"ai:\n" +
		"  bug_fix: false\n" +
		"  ci_debug: true\n" +
		"release:\n" +
		"  enabled: false\n"
	c, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if c.Repository.Visibility != "private" || c.Branches.Main.Approvals != 3 || len(c.Developers) != 2 || c.AI.BugFix || !c.AI.CIDebug || c.Release.Enabled {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestParseRejectsCalendarFields(t *testing.T) {
	if _, err := Parse([]byte("repository:\n  timezone: Asia/Shanghai\n")); err == nil {
		t.Fatal("expected calendar field to be rejected")
	}
}

func TestYAMLRoundTrip(t *testing.T) {
	original := Default()
	original.Developers = []string{"alice"}
	parsed, err := Parse([]byte(original.YAML()))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Developers[0] != "alice" || parsed.Branches.Main.Approvals != original.Branches.Main.Approvals {
		t.Fatalf("round trip mismatch: %+v", parsed)
	}
}

func TestEmptyDevelopersRoundTrip(t *testing.T) {
	if _, err := Parse([]byte(Default().YAML())); err != nil {
		t.Fatal(err)
	}
}

func TestParseV11AIConfig(t *testing.T) {
	input := "ai:\n" +
		"  enabled: true\n" +
		"  provider:\n" +
		"    type: openai-compatible\n" +
		"    base_url: https://example.invalid/v1\n" +
		"    model: test-model\n" +
		"    api_key: secret:REPOFLOW_AI_API_KEY\n" +
		"  analysis:\n" +
		"    issues: true\n" +
		"    rate_limit:\n" +
		"      max_per_user: 7\n" +
		"      window_minutes: 30\n" +
		"  policy:\n" +
		"    auto_fix_bugs: true\n" +
		"    min_confidence: 0.9\n" +
		"  ci_debug: true\n"
	c, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if c.AI.Provider.BaseURL != "https://example.invalid/v1" || c.AI.Provider.APIKey != "secret:REPOFLOW_AI_API_KEY" || c.AI.Policy.MinConfidence != 0.9 || c.AI.Analysis.MaxIssuesPerUser != 7 || c.AI.Analysis.RateWindowMinutes != 30 {
		t.Fatalf("unexpected AI config: %+v", c.AI)
	}
}

func TestRejectPlaintextAIKey(t *testing.T) {
	_, err := Parse([]byte("ai:\n  provider:\n    api_key: sk-live-secret\n"))
	if err == nil {
		t.Fatal("expected plaintext AI key to be rejected")
	}
}
