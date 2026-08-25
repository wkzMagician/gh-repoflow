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

func TestDesiredRulesetOmitsIntegrationID(t *testing.T) {
	payload := desiredRuleSetPayload(DesiredRuleSet{
		Name: "RepoFlow main", Branch: "main", Approvals: 2,
		RequiredChecks: []string{"test"},
	})
	rules := payload["rules"].([]map[string]interface{})
	for _, rule := range rules {
		if rule["type"] == "required_status_checks" {
			params := rule["parameters"].(map[string]interface{})
			checks := params["required_status_checks"].([]map[string]interface{})
			if len(checks) != 1 {
				t.Fatalf("expected 1 check, got %d", len(checks))
			}
			if _, hasIntegrationID := checks[0]["integration_id"]; hasIntegrationID {
				t.Fatalf("expected status check not to have integration_id, but found: %v", checks[0]["integration_id"])
			}
		}
	}
}

func TestIsForbidden(t *testing.T) {
	if !IsForbidden(assertErr("HTTP 403: Upgrade to GitHub Pro or make this repository public")) {
		t.Fatal("expected 403 error to be identified as forbidden")
	}
	if IsForbidden(assertErr("HTTP 404: Not Found")) {
		t.Fatal("expected 404 error not to be forbidden")
	}
}

type testError struct{ msg string }

func (e testError) Error() string { return e.msg }
func assertErr(msg string) error  { return testError{msg: msg} }
