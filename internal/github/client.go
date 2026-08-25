package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/repoflow/gh-repoflow/internal/command"
)

const apiAccept = "application/vnd.github+json"

type Client struct {
	Runner command.Runner
	Repo   string
}

type Repository struct {
	NameWithOwner    string `json:"full_name"`
	Visibility       string `json:"visibility"`
	DefaultBranch    string `json:"default_branch"`
	AllowSquashMerge bool   `json:"allow_squash_merge"`
	AllowMergeCommit bool   `json:"allow_merge_commit"`
	AllowRebaseMerge bool   `json:"allow_rebase_merge"`
	AllowAutoMerge   bool   `json:"allow_auto_merge"`
}

type Label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type RuleSetSummary struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Target      string `json:"target"`
	Enforcement string `json:"enforcement"`
}

type RuleSet struct {
	ID          int64                    `json:"id"`
	Name        string                   `json:"name"`
	Target      string                   `json:"target"`
	Enforcement string                   `json:"enforcement"`
	Conditions  map[string]interface{}   `json:"conditions"`
	Rules       []map[string]interface{} `json:"rules"`
}

type Collaborator struct {
	Permission string `json:"permission"`
}

type DesiredRuleSet struct {
	Name                 string
	Branch               string
	Approvals            int
	RequiredChecks       []string
	RequireReleaseSource bool
}

type ActionsVariable struct {
	Name  string
	Value string
}

func (c Client) Version(ctx context.Context) error {
	_, err := c.Runner.Run(ctx, "gh", "--version")
	return err
}

func (c Client) AuthStatus(ctx context.Context) error {
	_, err := c.Runner.Run(ctx, "gh", "auth", "status")
	return err
}

func (c Client) Discover(ctx context.Context) (string, error) {
	out, err := c.Runner.Run(ctx, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", fmt.Errorf("discover GitHub repository: %w", err)
	}
	repo := strings.TrimSpace(out)
	if repo == "" || !strings.Contains(repo, "/") {
		return "", errors.New("gh repo view returned no nameWithOwner")
	}
	return repo, nil
}

func (c Client) RepoInfo(ctx context.Context) (Repository, error) {
	var repo Repository
	err := c.api(ctx, "GET", c.endpoint(""), nil, &repo)
	if err != nil {
		return Repository{}, err
	}
	return repo, nil
}

func (c Client) CreateRepository(ctx context.Context, name, visibility string) error {
	flag := "--public"
	if visibility == "private" {
		flag = "--private"
	} else if visibility == "internal" {
		flag = "--internal"
	}
	_, err := c.Runner.Run(ctx, "gh", "repo", "create", name, flag, "--source", ".", "--remote", "origin")
	if err != nil {
		return fmt.Errorf("create GitHub repository: %w", err)
	}
	return nil
}

func (c Client) EnsureRepositorySettings(ctx context.Context, desired string) (bool, error) {
	actual, err := c.RepoInfo(ctx)
	if err != nil {
		return false, err
	}
	if actual.Visibility == desired && actual.DefaultBranch == "main" && actual.AllowSquashMerge && actual.AllowMergeCommit && !actual.AllowRebaseMerge && !actual.AllowAutoMerge {
		return false, nil
	}
	body := map[string]interface{}{
		"visibility":             desired,
		"default_branch":         "main",
		"allow_squash_merge":     true,
		"allow_merge_commit":     true,
		"allow_rebase_merge":     false,
		"allow_auto_merge":       false,
		"delete_branch_on_merge": false,
	}
	err = c.api(ctx, "PATCH", c.endpoint(""), body, nil)
	return true, err
}

func (c Client) BranchExists(ctx context.Context, branch string) (bool, error) {
	var result map[string]interface{}
	err := c.api(ctx, "GET", c.endpoint("/branches/"+url.PathEscape(branch)), nil, &result)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c Client) ListRulesets(ctx context.Context) ([]RuleSetSummary, error) {
	var rulesets []RuleSetSummary
	err := c.api(ctx, "GET", c.endpoint("/rulesets?includes_parents=true"), nil, &rulesets)
	return rulesets, err
}

func (c Client) Ruleset(ctx context.Context, id int64) (RuleSet, error) {
	var ruleset RuleSet
	err := c.api(ctx, "GET", c.endpoint(fmt.Sprintf("/rulesets/%d", id)), nil, &ruleset)
	return ruleset, err
}

func (c Client) EnsureRuleset(ctx context.Context, desired DesiredRuleSet) (bool, error) {
	summaries, err := c.ListRulesets(ctx)
	if err != nil {
		return false, err
	}
	var existing *RuleSet
	for _, summary := range summaries {
		if summary.Name == desired.Name {
			value, getErr := c.Ruleset(ctx, summary.ID)
			if getErr != nil {
				return false, getErr
			}
			existing = &value
			break
		}
	}
	payload := desiredRuleSetPayload(desired)
	if existing == nil {
		return true, c.api(ctx, "POST", c.endpoint("/rulesets"), payload, nil)
	}
	if rulesetMatches(*existing, payload) {
		return false, nil
	}
	return true, c.api(ctx, "PATCH", c.endpoint(fmt.Sprintf("/rulesets/%d", existing.ID)), payload, nil)
}

func (c Client) RulesetMatches(ctx context.Context, desired DesiredRuleSet) (bool, error) {
	summaries, err := c.ListRulesets(ctx)
	if err != nil {
		return false, err
	}
	for _, summary := range summaries {
		if summary.Name != desired.Name {
			continue
		}
		actual, err := c.Ruleset(ctx, summary.ID)
		if err != nil {
			return false, err
		}
		return rulesetMatches(actual, desiredRuleSetPayload(desired)), nil
	}
	return false, nil
}

func (c Client) EnsureLabel(ctx context.Context, desired Label) (bool, error) {
	var actual Label
	err := c.api(ctx, "GET", c.endpoint("/labels/"+url.PathEscape(desired.Name)), nil, &actual)
	if err != nil {
		if !isNotFound(err) {
			return false, err
		}
		return true, c.api(ctx, "POST", c.endpoint("/labels"), desired, nil)
	}
	if actual.Color == desired.Color && actual.Description == desired.Description {
		return false, nil
	}
	body := map[string]string{"new_name": desired.Name, "color": desired.Color, "description": desired.Description}
	return true, c.api(ctx, "PATCH", c.endpoint("/labels/"+url.PathEscape(desired.Name)), body, nil)
}

func (c Client) EnsureCollaborator(ctx context.Context, username, permission string) (bool, error) {
	var actual Collaborator
	err := c.api(ctx, "GET", c.endpoint("/collaborators/"+url.PathEscape(username)+"/permission"), nil, &actual)
	if err == nil && PermissionSatisfies(actual.Permission, permission) {
		return false, nil
	}
	if err != nil && !isNotFound(err) {
		return false, err
	}
	body := map[string]string{"permission": permission}
	return true, c.api(ctx, "PUT", c.endpoint("/collaborators/"+url.PathEscape(username)), body, nil)
}

func (c Client) CollaboratorPermission(ctx context.Context, username string) (string, error) {
	var actual Collaborator
	err := c.api(ctx, "GET", c.endpoint("/collaborators/"+url.PathEscape(username)+"/permission"), nil, &actual)
	return actual.Permission, err
}

func PermissionSatisfies(actual, desired string) bool {
	return permissionRank(actual) >= permissionRank(desired)
}

func (c Client) Label(ctx context.Context, name string) (Label, error) {
	var label Label
	err := c.api(ctx, "GET", c.endpoint("/labels/"+url.PathEscape(name)), nil, &label)
	return label, err
}

func (c Client) EnsureVariable(ctx context.Context, name, value string) (bool, error) {
	var actual ActionsVariable
	err := c.api(ctx, "GET", c.endpoint("/actions/variables/"+url.PathEscape(name)), nil, &actual)
	body := map[string]string{"name": name, "value": value}
	if err == nil {
		if actual.Value == value {
			return false, nil
		}
		return true, c.api(ctx, "PUT", c.endpoint("/actions/variables/"+url.PathEscape(name)), body, nil)
	}
	if !isNotFound(err) {
		return false, err
	}
	return true, c.api(ctx, "POST", c.endpoint("/actions/variables"), body, nil)
}

func (c Client) Variable(ctx context.Context, name string) (ActionsVariable, error) {
	var variable ActionsVariable
	err := c.api(ctx, "GET", c.endpoint("/actions/variables/"+url.PathEscape(name)), nil, &variable)
	return variable, err
}

type apiErrorResponse struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
	Errors           []any  `json:"errors"`
}

func (c Client) api(ctx context.Context, method, endpoint string, body interface{}, result interface{}) error {
	args := []string{"api", endpoint, "--method", method, "--header", "Accept: " + apiAccept}
	var out string
	var runErr error
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		args = append(args, "--input", "-")
		out, runErr = c.Runner.RunInput(ctx, data, "gh", args...)
	} else {
		out, runErr = c.Runner.Run(ctx, "gh", args...)
	}
	if runErr != nil {
		// Try to parse GitHub API error json from output or error string if available
		var apiErr apiErrorResponse
		if err := json.Unmarshal([]byte(out), &apiErr); err == nil && apiErr.Message != "" {
			detail := apiErr.Message
			if len(apiErr.Errors) > 0 {
				errBytes, _ := json.Marshal(apiErr.Errors)
				detail = fmt.Sprintf("%s: %s", apiErr.Message, string(errBytes))
			}
			return fmt.Errorf("GitHub API %s %s: %s (%w)", method, endpoint, detail, runErr)
		}
		return fmt.Errorf("GitHub API %s %s: %w", method, endpoint, runErr)
	}
	if result != nil && strings.TrimSpace(out) != "" {
		if err := json.Unmarshal([]byte(out), result); err != nil {
			return fmt.Errorf("decode GitHub API response: %w", err)
		}
	}
	return nil
}

func (c Client) endpoint(suffix string) string {
	return "/repos/" + c.Repo + suffix
}

func isNotFound(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "404") || strings.Contains(text, "not found")
}

func IsForbidden(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "403") || strings.Contains(text, "upgrade to github pro") || strings.Contains(text, "forbidden")
}

func desiredRuleSetPayload(desired DesiredRuleSet) map[string]interface{} {
	statusChecks := make([]map[string]interface{}, 0, len(desired.RequiredChecks))
	for _, check := range desired.RequiredChecks {
		statusChecks = append(statusChecks, map[string]interface{}{"context": check})
	}
	rules := []map[string]interface{}{
		{"type": "deletion"},
		{"type": "non_fast_forward"},
		{
			"type": "pull_request",
			"parameters": map[string]interface{}{
				"required_approving_review_count":   desired.Approvals,
				"dismiss_stale_reviews_on_push":     true,
				"require_code_owner_review":         false,
				"require_last_push_approval":        false,
				"required_review_thread_resolution": true,
			},
		},
	}
	if len(statusChecks) > 0 {
		rules = append(rules, map[string]interface{}{
			"type": "required_status_checks",
			"parameters": map[string]interface{}{
				"required_status_checks":               statusChecks,
				"strict_required_status_checks_policy": true,
			},
		})
	}
	return map[string]interface{}{
		"name":        desired.Name,
		"target":      "branch",
		"enforcement": "active",
		"conditions": map[string]interface{}{
			"ref_name": map[string]interface{}{
				"include": []string{"refs/heads/" + desired.Branch},
				"exclude": []string{},
			},
		},
		"rules": rules,
	}
}

func rulesetMatches(actual RuleSet, desired map[string]interface{}) bool {
	if actual.Name != stringValue(desired["name"]) || actual.Target != stringValue(desired["target"]) || actual.Enforcement != stringValue(desired["enforcement"]) {
		return false
	}
	desiredConditions := desired["conditions"].(map[string]interface{})
	actualRef, _ := actual.Conditions["ref_name"].(map[string]interface{})
	desiredRef := desiredConditions["ref_name"].(map[string]interface{})
	if !sameStringSlice(actualRef["include"], desiredRef["include"]) {
		return false
	}
	desiredRules := desired["rules"].([]map[string]interface{})
	for _, rule := range desiredRules {
		typ := stringValue(rule["type"])
		found := false
		for _, existing := range actual.Rules {
			if stringValue(existing["type"]) != typ {
				continue
			}
			found = true
			if typ == "pull_request" {
				want := rule["parameters"].(map[string]interface{})
				have, _ := existing["parameters"].(map[string]interface{})
				if !parametersMatch(have, want) {
					return false
				}
			}
			if typ == "required_status_checks" {
				want := rule["parameters"].(map[string]interface{})
				have, _ := existing["parameters"].(map[string]interface{})
				if !sameStatusChecks(have["required_status_checks"], want["required_status_checks"]) {
					return false
				}
			}
			break
		}
		if !found {
			return false
		}
	}
	return true
}

func isNumber(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

func parametersMatch(actual, desired map[string]interface{}) bool {
	for key, expected := range desired {
		if key == "required_status_checks" {
			continue
		}
		actVal, hasAct := actual[key]
		if !hasAct {
			return false
		}
		if isNumber(actVal) || isNumber(expected) {
			if intValue(actVal) != intValue(expected) {
				return false
			}
			continue
		}
		if !reflect.DeepEqual(actVal, expected) {
			return false
		}
	}
	return true
}

func sameStatusChecks(a, b interface{}) bool {
	normalize := func(value interface{}) []string {
		result := make([]string, 0)
		switch items := value.(type) {
		case []interface{}:
			for _, item := range items {
				if object, ok := item.(map[string]interface{}); ok {
					result = append(result, stringValue(object["context"]))
				}
			}
		case []map[string]interface{}:
			for _, object := range items {
				result = append(result, stringValue(object["context"]))
			}
		}
		sort.Strings(result)
		return result
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}

func sameStringSlice(a, b interface{}) bool {
	toStrings := func(value interface{}) []string {
		switch items := value.(type) {
		case []interface{}:
			result := make([]string, 0, len(items))
			for _, item := range items {
				result = append(result, stringValue(item))
			}
			return result
		case []string:
			return items
		default:
			return nil
		}
	}
	left, right := toStrings(a), toStrings(b)
	sort.Strings(left)
	sort.Strings(right)
	return reflect.DeepEqual(left, right)
}

func stringValue(value interface{}) string {
	text, _ := value.(string)
	return text
}

func intValue(value interface{}) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	default:
		return 0
	}
}

func permissionRank(permission string) int {
	switch strings.ToLower(permission) {
	case "pull":
		return 1
	case "triage":
		return 2
	case "push", "write":
		return 3
	case "maintain":
		return 4
	case "admin", "owner":
		return 5
	default:
		return 0
	}
}

// Used by tests and diagnostics to produce stable JSON for desired payloads.
func marshalStable(value interface{}) string {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	return strings.TrimSpace(b.String())
}
