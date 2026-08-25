package github

import "testing"

func TestDesiredRulesetContainsReleaseSourceCheck(t *testing.T) {
	payload := desiredRuleSetPayload(DesiredRuleSet{
		Name: "RepoFlow main", Branch: "main", Approvals: 2,
		RequiredChecks: []string{"test", "release-source"},
	})
	rules := payload["rules"].([]map[string]interface{})
	var found map[string]interface{}
	for _, rule := range rules {
		if rule["type"] == "required_status_checks" {
			found = rule
		}
	}
	if found == nil {
		t.Fatal("expected required status checks rule")
	}
	parameters := found["parameters"].(map[string]interface{})
	checks := parameters["required_status_checks"].([]map[string]interface{})
	if len(checks) != 2 || checks[1]["context"] != "release-source" {
		t.Fatalf("unexpected checks: %#v", checks)
	}
}

func TestRulesetMatchesApprovalAndChecks(t *testing.T) {
	desired := desiredRuleSetPayload(DesiredRuleSet{
		Name: "RepoFlow dev", Branch: "dev", Approvals: 1,
		RequiredChecks: []string{"test"},
	})
	actual := RuleSet{
		Name: "RepoFlow dev", Target: "branch", Enforcement: "active",
		Conditions: map[string]interface{}{
			"ref_name": map[string]interface{}{"include": []interface{}{"refs/heads/dev"}},
		},
		Rules: []map[string]interface{}{
			{"type": "deletion"},
			{"type": "non_fast_forward"},
			{"type": "pull_request", "parameters": map[string]interface{}{
				"required_approving_review_count":   float64(1),
				"dismiss_stale_reviews_on_push":     true,
				"require_code_owner_review":         false,
				"require_last_push_approval":        false,
				"required_review_thread_resolution": true,
			}},
			{"type": "required_status_checks", "parameters": map[string]interface{}{
				"required_status_checks": []interface{}{map[string]interface{}{"context": "test"}},
			}},
		},
	}
	if !rulesetMatches(actual, desired) {
		t.Fatal("expected ruleset to match")
	}
}
