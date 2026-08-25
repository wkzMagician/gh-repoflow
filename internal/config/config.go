package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const Path = ".github/repoflow.yml"

type Config struct {
	Repository RepositoryConfig
	Branches   BranchConfig
	Developers []string
	CI         CIConfig
	AI         AIConfig
	Release    ReleaseConfig
}

type RepositoryConfig struct {
	Visibility string
}

type BranchConfig struct {
	Main BranchPolicy
	Dev  BranchPolicy
}

type BranchPolicy struct {
	Approvals int
}

type CIConfig struct{ Enabled bool }

type AIConfig struct {
	Enabled  bool
	Provider AIProviderConfig
	Analysis AIAnalysisConfig
	Policy   AIPolicyConfig
	CIDebug  bool
	// BugFix is retained as a read-compatible alias for pre-v1.1 files.
	BugFix bool
}

type ReleaseConfig struct{ Enabled bool }

type AIProviderConfig struct {
	Type    string
	BaseURL string
	Model   string
	APIKey  string
}

type AIAnalysisConfig struct {
	Issues            bool
	MaxIssuesPerUser  int
	RateWindowMinutes int
}

type AIPolicyConfig struct {
	AutoFixBugs   bool
	MinConfidence float64
}

func Default() Config {
	return Config{
		Repository: RepositoryConfig{Visibility: "public"},
		Branches: BranchConfig{
			Main: BranchPolicy{Approvals: 0},
			Dev:  BranchPolicy{Approvals: 0},
		},
		CI: CIConfig{Enabled: true},
		AI: AIConfig{
			Enabled: true,
			Provider: AIProviderConfig{
				Type: "openai-compatible", BaseURL: "https://openrouter.ai/api/v1",
				Model: "xxx", APIKey: "secret:REPOFLOW_AI_API_KEY",
			},
			Analysis: AIAnalysisConfig{Issues: true, MaxIssuesPerUser: 5, RateWindowMinutes: 60},
			Policy:   AIPolicyConfig{AutoFixBugs: true, MinConfidence: 0.85},
			CIDebug:  true,
			BugFix:   true,
		},
		Release: ReleaseConfig{Enabled: true},
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse intentionally supports the small, documented RepoFlow schema without
// adding a YAML dependency. It accepts comments, quoted scalars, booleans,
// integers, and the list of developer usernames used by V1.
func Parse(data []byte) (Config, error) {
	c := Default()
	section := ""
	subsection := ""
	subsection2 := ""
	branch := ""
	seen := map[string]bool{}
	s := bufio.NewScanner(strings.NewReader(string(data)))
	lineNo := 0
	for s.Scan() {
		lineNo++
		raw := s.Text()
		line := strings.TrimSpace(stripComment(raw))
		if line == "" || line == "---" {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 && line == "developers: []" {
			section = "developers"
			branch = ""
			continue
		}
		if indent == 0 && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			subsection = ""
			subsection2 = ""
			branch = ""
			if !knownTopLevel(section) {
				return Config{}, fmt.Errorf("line %d: unsupported section %q", lineNo, section)
			}
			continue
		}
		if section == "developers" && strings.HasPrefix(line, "-") {
			value := cleanScalar(strings.TrimSpace(strings.TrimPrefix(line, "-")))
			if value == "" {
				return Config{}, fmt.Errorf("line %d: developer username cannot be empty", lineNo)
			}
			c.Developers = append(c.Developers, value)
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Config{}, fmt.Errorf("line %d: expected key: value", lineNo)
		}
		key = strings.TrimSpace(key)
		value = cleanScalar(strings.TrimSpace(value))
		if isForbidden(key) {
			return Config{}, fmt.Errorf("line %d: %q is not supported; RepoFlow is event-driven", lineNo, key)
		}
		if section == "branches" && indent <= 2 && value == "" {
			branch = key
			if branch != "main" && branch != "dev" {
				return Config{}, fmt.Errorf("line %d: unsupported branch %q", lineNo, branch)
			}
			continue
		}
		if section == "ai" && indent <= 2 && value == "" {
			switch key {
			case "provider", "analysis", "policy":
				subsection = key
				subsection2 = ""
				continue
			default:
				return Config{}, fmt.Errorf("line %d: unsupported AI section %q", lineNo, key)
			}
		}
		if section == "ai" && indent <= 2 && value != "" {
			subsection = ""
			subsection2 = ""
		}
		if section == "ai" && subsection == "analysis" && indent <= 4 && value != "" {
			subsection2 = ""
		}
		if section == "ai" && subsection == "analysis" && indent <= 4 && value == "" {
			if key != "rate_limit" {
				return Config{}, fmt.Errorf("line %d: unsupported AI analysis section %q", lineNo, key)
			}
			subsection2 = key
			continue
		}
		pathKey := section + "." + key
		if branch != "" {
			pathKey = section + "." + branch + "." + key
		} else if subsection != "" {
			pathKey = section + "." + subsection + "." + key
			if subsection2 != "" {
				pathKey = section + "." + subsection + "." + subsection2 + "." + key
			}
		}
		if seen[pathKey] {
			return Config{}, fmt.Errorf("line %d: duplicate key %q", lineNo, pathKey)
		}
		seen[pathKey] = true
		var err error
		switch pathKey {
		case "repository.visibility":
			if value != "public" && value != "private" && value != "internal" {
				return Config{}, fmt.Errorf("line %d: visibility must be public, private, or internal", lineNo)
			}
			c.Repository.Visibility = value
		case "branches.main.approvals":
			c.Branches.Main.Approvals, err = parseNonNegative(value)
		case "branches.dev.approvals":
			c.Branches.Dev.Approvals, err = parseNonNegative(value)
		case "ci.enabled":
			c.CI.Enabled, err = strconv.ParseBool(value)
		case "ai.enabled":
			c.AI.Enabled, err = strconv.ParseBool(value)
		case "ai.provider.type":
			if value != "openai-compatible" {
				return Config{}, fmt.Errorf("line %d: only openai-compatible AI provider is supported in V1", lineNo)
			}
			c.AI.Provider.Type = value
		case "ai.provider.base_url":
			c.AI.Provider.BaseURL = value
		case "ai.provider.model":
			c.AI.Provider.Model = value
		case "ai.provider.api_key":
			if value != "secret:REPOFLOW_AI_API_KEY" {
				return Config{}, fmt.Errorf("line %d: ai.provider.api_key must be secret:REPOFLOW_AI_API_KEY in V1", lineNo)
			}
			c.AI.Provider.APIKey = value
		case "ai.analysis.issues":
			c.AI.Analysis.Issues, err = strconv.ParseBool(value)
		case "ai.analysis.rate_limit.max_per_user":
			c.AI.Analysis.MaxIssuesPerUser, err = parsePositive(value)
		case "ai.analysis.rate_limit.window_minutes":
			c.AI.Analysis.RateWindowMinutes, err = parsePositive(value)
		case "ai.policy.auto_fix_bugs":
			c.AI.Policy.AutoFixBugs, err = strconv.ParseBool(value)
			c.AI.BugFix = c.AI.Policy.AutoFixBugs
		case "ai.policy.min_confidence":
			c.AI.Policy.MinConfidence, err = strconv.ParseFloat(value, 64)
			if err == nil && (c.AI.Policy.MinConfidence < 0 || c.AI.Policy.MinConfidence > 1) {
				err = errors.New("must be between 0 and 1")
			}
		case "ai.bug_fix":
			c.AI.BugFix, err = strconv.ParseBool(value)
			c.AI.Policy.AutoFixBugs = c.AI.BugFix
		case "ai.ci_debug":
			c.AI.CIDebug, err = strconv.ParseBool(value)
		case "release.enabled":
			c.Release.Enabled, err = strconv.ParseBool(value)
		default:
			return Config{}, fmt.Errorf("line %d: unsupported key %q", lineNo, pathKey)
		}
		if err != nil {
			return Config{}, fmt.Errorf("line %d: invalid value for %s: %w", lineNo, pathKey, err)
		}
	}
	if err := s.Err(); err != nil {
		return Config{}, err
	}
	if c.Branches.Main.Approvals < 0 || c.Branches.Dev.Approvals < 0 {
		return Config{}, errors.New("branches.main.approvals and branches.dev.approvals must be non-negative")
	}
	if c.AI.Provider.Type == "" {
		c.AI.Provider.Type = "openai-compatible"
	}
	if c.AI.Provider.Type != "openai-compatible" {
		return Config{}, errors.New("only openai-compatible AI provider is supported in V1")
	}
	if strings.TrimSpace(c.AI.Provider.BaseURL) == "" || strings.TrimSpace(c.AI.Provider.Model) == "" {
		return Config{}, errors.New("AI provider base_url and model are required")
	}
	if c.AI.Provider.APIKey == "" {
		c.AI.Provider.APIKey = "secret:REPOFLOW_AI_API_KEY"
	}
	if c.AI.Provider.APIKey != "secret:REPOFLOW_AI_API_KEY" {
		return Config{}, errors.New("AI API keys must use the secret:REPOFLOW_AI_API_KEY reference")
	}
	if c.AI.Analysis.MaxIssuesPerUser < 1 || c.AI.Analysis.RateWindowMinutes < 1 {
		return Config{}, errors.New("AI analysis rate limit must allow at least 1 issue and have a positive window")
	}
	c.AI.BugFix = c.AI.Policy.AutoFixBugs
	return c, nil
}

func (c Config) YAML() string {
	var b strings.Builder
	fmt.Fprintf(&b, "repository:\n  visibility: %s\n\n", c.Repository.Visibility)
	fmt.Fprintf(&b, "branches:\n  main:\n    approvals: %d\n  dev:\n    approvals: %d\n\n", c.Branches.Main.Approvals, c.Branches.Dev.Approvals)
	if len(c.Developers) == 0 {
		b.WriteString("developers: []\n")
	} else {
		b.WriteString("developers:\n")
		for _, developer := range c.Developers {
			fmt.Fprintf(&b, "  - %s\n", developer)
		}
	}
	fmt.Fprintf(&b, "\nci:\n  enabled: %t\n\nai:\n  enabled: %t\n  provider:\n    type: %s\n    base_url: %s\n    model: %s\n    api_key: %s\n  analysis:\n    issues: %t\n    rate_limit:\n      max_per_user: %d\n      window_minutes: %d\n  policy:\n    auto_fix_bugs: %t\n    min_confidence: %.2f\n  ci_debug: %t\n\nrelease:\n  enabled: %t\n", c.CI.Enabled, c.AI.Enabled, c.AI.Provider.Type, c.AI.Provider.BaseURL, c.AI.Provider.Model, c.AI.Provider.APIKey, c.AI.Analysis.Issues, c.AI.Analysis.MaxIssuesPerUser, c.AI.Analysis.RateWindowMinutes, c.AI.Policy.AutoFixBugs, c.AI.Policy.MinConfidence, c.AI.CIDebug, c.Release.Enabled)
	return b.String()
}

func parseNonNegative(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, errors.New("must be a non-negative integer")
	}
	return n, nil
}

func parsePositive(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, errors.New("must be a positive integer")
	}
	return n, nil
}

func cleanScalar(value string) string {
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
		return value[1 : len(value)-1]
	}
	return value
}

func stripComment(line string) string {
	quoted := byte(0)
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'', '"':
			if quoted == 0 {
				quoted = line[i]
			} else if quoted == line[i] {
				quoted = 0
			}
		case '#':
			if quoted == 0 {
				return line[:i]
			}
		}
	}
	return line
}

func knownTopLevel(section string) bool {
	switch section {
	case "repository", "branches", "developers", "ci", "ai", "release":
		return true
	default:
		return false
	}
}

func isForbidden(key string) bool {
	switch strings.ToLower(key) {
	case "timezone", "schedule", "cycle", "week", "monday", "friday", "saturday", "sunday":
		return true
	default:
		return false
	}
}
