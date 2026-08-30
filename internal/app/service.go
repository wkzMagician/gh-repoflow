package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/repoflow/gh-repoflow/internal/command"
	"github.com/repoflow/gh-repoflow/internal/config"
	"github.com/repoflow/gh-repoflow/internal/git"
	"github.com/repoflow/gh-repoflow/internal/github"
	repoTemplate "github.com/repoflow/gh-repoflow/internal/template"
)

type Service struct {
	Root   string
	Runner command.Runner
	Git    git.Client
	GitHub github.Client
	Out    io.Writer
}

type InitOptions struct {
	Name       string
	Visibility string
	Topic      string
}

func New(root string, runner command.Runner, out io.Writer) Service {
	return Service{
		Root:   root,
		Runner: runner,
		Git:    git.Client{Runner: runner},
		GitHub: github.Client{Runner: runner},
		Out:    out,
	}
}

func (s Service) Bootstrap(ctx context.Context, options InitOptions) error {
	if err := s.Git.Version(ctx); err != nil {
		return fmt.Errorf("git is required: %w", err)
	}
	if err := s.GitHub.Version(ctx); err != nil {
		return fmt.Errorf("gh is required: %w", err)
	}
	if err := s.GitHub.AuthStatus(ctx); err != nil {
		return fmt.Errorf("GitHub authentication is required; run gh auth login: %w", err)
	}
	existingRepository := s.Git.IsRepository(ctx)
	if existingRepository && s.Git.HasCommits(ctx) {
		dirty, err := s.Git.HasUncommittedChanges(ctx)
		if err != nil {
			return fmt.Errorf("inspect existing git worktree: %w", err)
		}
		if dirty {
			return errors.New("working tree has uncommitted changes; commit or stash them before running repoflow init")
		}
	}
	if !existingRepository {
		if err := s.Git.Init(ctx); err != nil {
			return fmt.Errorf("initialize git repository: %w", err)
		}
	}
	if err := s.Git.EnsureMain(ctx); err != nil {
		return err
	}
	cfg, err := s.ensureConfig(options.Visibility)
	if err != nil {
		return err
	}
	if _, err := repoTemplate.Ensure(s.Root); err != nil {
		return err
	}
	if err := s.Git.AddAll(ctx); err != nil {
		return fmt.Errorf("stage initial files: %w", err)
	}
	staged, err := s.Git.HasStagedChanges(ctx)
	if err != nil {
		return fmt.Errorf("inspect staged files: %w", err)
	}
	if staged {
		if err := s.Git.Commit(ctx, "chore: initialize RepoFlow"); err != nil {
			return fmt.Errorf("create initial commit: %w", err)
		}
	}
	name := options.Name
	if name == "" {
		name = filepath.Base(s.Root)
	}
	needCreate := !s.Git.HasOrigin(ctx)
	if !needCreate {
		// Resolve the origin before querying repository metadata. RepoInfo uses
		// the discovered owner/name and cannot safely query with an empty repo.
		if repo, discoverErr := s.GitHub.Discover(ctx); discoverErr != nil {
			if isNotFound(discoverErr) || strings.Contains(discoverErr.Error(), "Could not resolve") {
				needCreate = true
			} else {
				return discoverErr
			}
		} else {
			s.GitHub.Repo = repo
			if _, err := s.GitHub.RepoInfo(ctx); err != nil && (isNotFound(err) || strings.Contains(err.Error(), "Could not resolve")) {
				needCreate = true
			}
		}
	}
	if needCreate {
		if err := s.GitHub.CreateRepository(ctx, name, cfg.Repository.Visibility); err != nil {
			return err
		}
	}
	repo, err := s.GitHub.Discover(ctx)
	if err != nil {
		return err
	}
	s.GitHub.Repo = repo
	if options.Topic != "" {
		if err := s.GitHub.EnsureTopic(ctx, options.Topic); err != nil {
			return fmt.Errorf("ensure repository topic: %w", err)
		}
	}
	if err := s.Git.Push(ctx, "main"); err != nil {
		return fmt.Errorf("push main: %w", err)
	}
	if err := s.Git.EnsureDev(ctx); err != nil {
		return fmt.Errorf("create dev branch: %w", err)
	}
	if err := s.Git.Push(ctx, "dev"); err != nil {
		return fmt.Errorf("push dev: %w", err)
	}
	if current, _ := s.Git.CurrentBranch(ctx); current != "main" {
		if err := s.Git.Checkout(ctx, "main"); err != nil {
			return fmt.Errorf("return to main: %w", err)
		}
	}
	if err := s.Apply(ctx, cfg); err != nil {
		return err
	}
	if cfg.AI.Enabled {
		fmt.Fprintln(s.Out, "AI workflows require the GitHub secret REPOFLOW_AI_API_KEY.")
		fmt.Fprintln(s.Out, "Create it with: gh secret set REPOFLOW_AI_API_KEY")
	}
	return s.Check(ctx, cfg)
}

func (s Service) Apply(ctx context.Context, cfg config.Config) error {
	if s.GitHub.Repo == "" {
		repo, err := s.GitHub.Discover(ctx)
		if err != nil {
			return err
		}
		s.GitHub.Repo = repo
	}
	if _, err := repoTemplate.Ensure(s.Root); err != nil {
		return err
	}
	if err := s.ensureRemoteBranches(ctx); err != nil {
		return err
	}
	if _, err := s.GitHub.EnsureRepositorySettings(ctx, cfg.Repository.Visibility); err != nil {
		return fmt.Errorf("apply repository settings: %w", err)
	}
	checks := []string{}
	if cfg.CI.Enabled {
		checks = append(checks, "test")
	}
	mainChecks := append([]string(nil), checks...)
	if cfg.Release.Enabled {
		mainChecks = append(mainChecks, "release-source")
	}
	if _, err := s.GitHub.EnsureRuleset(ctx, github.DesiredRuleSet{
		Name: "RepoFlow main", Branch: "main", Approvals: cfg.Branches.Main.Approvals,
		RequiredChecks: mainChecks, RequireReleaseSource: true,
	}); err != nil {
		if github.IsForbidden(err) {
			fmt.Fprintln(s.Out, "Notice: branch rulesets are not available on this repository (requires GitHub Pro/Team for private repositories). Skipping.")
		} else {
			return fmt.Errorf("apply main ruleset: %w", err)
		}
	}
	if _, err := s.GitHub.EnsureRuleset(ctx, github.DesiredRuleSet{
		Name: "RepoFlow dev", Branch: "dev", Approvals: cfg.Branches.Dev.Approvals,
		RequiredChecks: checks,
	}); err != nil {
		if github.IsForbidden(err) {
			// Already warned on main
		} else {
			return fmt.Errorf("apply dev ruleset: %w", err)
		}
	}
	for _, developer := range cfg.Developers {
		if _, err := s.GitHub.EnsureCollaborator(ctx, developer, "push"); err != nil {
			return fmt.Errorf("apply collaborator %s: %w", developer, err)
		}
	}
	for _, label := range labels() {
		if _, err := s.GitHub.EnsureLabel(ctx, label); err != nil {
			return fmt.Errorf("apply label %s: %w", label.Name, err)
		}
	}
	if cfg.AI.Enabled {
		variables := map[string]string{
			"REPOFLOW_AI_BASE_URL":            cfg.AI.Provider.BaseURL,
			"REPOFLOW_AI_MODEL":               cfg.AI.Provider.Model,
			"REPOFLOW_AI_MIN_CONFIDENCE":      fmt.Sprintf("%.2f", cfg.AI.Policy.MinConfidence),
			"REPOFLOW_AI_MAX_ISSUES_PER_USER": fmt.Sprintf("%d", cfg.AI.Analysis.MaxIssuesPerUser),
			"REPOFLOW_AI_RATE_WINDOW_MINUTES": fmt.Sprintf("%d", cfg.AI.Analysis.RateWindowMinutes),
		}
		for name, value := range variables {
			if _, err := s.GitHub.EnsureVariable(ctx, name, value); err != nil {
				return fmt.Errorf("apply Actions variable %s: %w", name, err)
			}
		}
	}
	fmt.Fprintln(s.Out, "RepoFlow applied.")
	return nil
}

func (s Service) Check(ctx context.Context, cfg config.Config) error {
	if s.GitHub.Repo == "" {
		repo, err := s.GitHub.Discover(ctx)
		if err != nil {
			return err
		}
		s.GitHub.Repo = repo
	}
	ok := true
	check := func(name string, err error) {
		if err != nil {
			ok = false
			fmt.Fprintf(s.Out, "%-24s ✗ %s\n", name, err)
			return
		}
		fmt.Fprintf(s.Out, "%-24s ✓\n", name)
	}
	repository, err := s.GitHub.RepoInfo(ctx)
	if err != nil {
		check("Repository", err)
	} else {
		check("Repository", nil)
		check("repository visibility", expectEqual(repository.Visibility, cfg.Repository.Visibility))
		check("default branch", expectEqual(repository.DefaultBranch, "main"))
		check("merge strategy", expectMergeSettings(repository))
	}
	for _, branch := range []string{"main", "dev"} {
		exists, branchErr := s.GitHub.BranchExists(ctx, branch)
		if branchErr != nil {
			check(branch+" branch", branchErr)
		} else if !exists {
			check(branch+" branch", fmt.Errorf("branch does not exist"))
		} else {
			check(branch+" branch", nil)
		}
	}
	mainChecks := []string{}
	if cfg.Release.Enabled {
		mainChecks = append(mainChecks, "release-source")
	}
	devChecks := []string{}
	if cfg.CI.Enabled {
		mainChecks = append([]string{"test"}, mainChecks...)
		devChecks = []string{"test"}
	}
	mainMatch, mainErr := s.GitHub.RulesetMatches(ctx, github.DesiredRuleSet{
		Name: "RepoFlow main", Branch: "main", Approvals: cfg.Branches.Main.Approvals, RequiredChecks: mainChecks,
	})
	if mainErr != nil {
		if github.IsForbidden(mainErr) {
			fmt.Fprintf(s.Out, "%-24s - skipped (requires GitHub Pro for private repositories)\n", "main ruleset")
		} else {
			check("main ruleset", mainErr)
		}
	} else if !mainMatch {
		check("main ruleset", errors.New("missing or differs from expected protected ruleset"))
	} else {
		check("main ruleset", nil)
	}
	devMatch, devErr := s.GitHub.RulesetMatches(ctx, github.DesiredRuleSet{
		Name: "RepoFlow dev", Branch: "dev", Approvals: cfg.Branches.Dev.Approvals, RequiredChecks: devChecks,
	})
	if devErr != nil {
		if github.IsForbidden(devErr) {
			fmt.Fprintf(s.Out, "%-24s - skipped (requires GitHub Pro for private repositories)\n", "dev ruleset")
		} else {
			check("dev ruleset", devErr)
		}
	} else if !devMatch {
		check("dev ruleset", errors.New("missing or differs from expected protected ruleset"))
	} else {
		check("dev ruleset", nil)
	}
	for _, developer := range cfg.Developers {
		permission, permissionErr := s.GitHub.CollaboratorPermission(ctx, developer)
		if permissionErr != nil {
			check("collaborator "+developer, permissionErr)
		} else {
			if github.PermissionSatisfies(permission, "push") {
				check("collaborator "+developer, nil)
			} else {
				check("collaborator "+developer, fmt.Errorf("expected push or stronger permission, got %s", permission))
			}
		}
	}
	for _, label := range labels() {
		actual, labelErr := s.GitHub.Label(ctx, label.Name)
		if labelErr != nil {
			check("label "+label.Name, labelErr)
		} else if actual.Color != label.Color || actual.Description != label.Description {
			check("label "+label.Name, errors.New("label color or description differs"))
		} else {
			check("label "+label.Name, nil)
		}
	}
	if cfg.CI.Enabled {
		check("CI workflow", localFile(s.Root, ".github/workflows/ci.yml"))
	}
	if cfg.AI.Enabled && cfg.AI.Analysis.Issues {
		check("AI issue workflow", localFile(s.Root, ".github/workflows/ai-issue-analysis.yml"))
	}
	if cfg.AI.Enabled && cfg.AI.Policy.AutoFixBugs {
		check("AI bug-fix workflow", localFile(s.Root, ".github/workflows/ai-bug-fix.yml"))
	}
	if cfg.AI.Enabled && cfg.AI.CIDebug {
		check("AI debug workflow", localFile(s.Root, ".github/workflows/ai-debug.yml"))
	}
	if cfg.AI.Enabled {
		for name, expected := range map[string]string{
			"REPOFLOW_AI_BASE_URL":            cfg.AI.Provider.BaseURL,
			"REPOFLOW_AI_MODEL":               cfg.AI.Provider.Model,
			"REPOFLOW_AI_MIN_CONFIDENCE":      fmt.Sprintf("%.2f", cfg.AI.Policy.MinConfidence),
			"REPOFLOW_AI_MAX_ISSUES_PER_USER": fmt.Sprintf("%d", cfg.AI.Analysis.MaxIssuesPerUser),
			"REPOFLOW_AI_RATE_WINDOW_MINUTES": fmt.Sprintf("%d", cfg.AI.Analysis.RateWindowMinutes),
		} {
			variable, variableErr := s.GitHub.Variable(ctx, name)
			if variableErr != nil {
				check("AI variable "+name, variableErr)
			} else {
				check("AI variable "+name, expectEqual(variable.Value, expected))
			}
		}
	}
	if cfg.Release.Enabled {
		check("release workflow", localFile(s.Root, ".github/workflows/release.yml"))
	}
	check("PR template", localFile(s.Root, ".github/pull_request_template.md"))
	check("issue templates", localFile(s.Root, ".github/ISSUE_TEMPLATE/bug.yml", ".github/ISSUE_TEMPLATE/feature.yml"))
	if !ok {
		return errors.New("RepoFlow check failed")
	}
	fmt.Fprintln(s.Out, "RepoFlow check passed.")
	return nil
}

func (s Service) ensureConfig(visibility string) (config.Config, error) {
	path := filepath.Join(s.Root, filepath.FromSlash(config.Path))
	if data, err := os.ReadFile(path); err == nil {
		cfg, parseErr := config.Parse(data)
		if parseErr != nil {
			return config.Config{}, parseErr
		}
		if visibility != "" && cfg.Repository.Visibility != visibility {
			cfg.Repository.Visibility = visibility
			if err := os.WriteFile(path, []byte(cfg.YAML()), 0o644); err != nil {
				return config.Config{}, fmt.Errorf("update %s: %w", config.Path, err)
			}
		}
		return cfg, nil
	} else if !os.IsNotExist(err) {
		return config.Config{}, err
	}
	cfg := config.Default()
	if visibility != "" {
		cfg.Repository.Visibility = visibility
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return config.Config{}, err
	}
	if err := os.WriteFile(path, []byte(cfg.YAML()), 0o644); err != nil {
		return config.Config{}, fmt.Errorf("write %s: %w", config.Path, err)
	}
	return cfg, nil
}

func (s Service) ensureRemoteBranches(ctx context.Context) error {
	mainExists, err := s.GitHub.BranchExists(ctx, "main")
	if err != nil {
		return fmt.Errorf("check main branch: %w", err)
	}
	if !mainExists {
		return errors.New("remote main branch does not exist; refusing to create governance on an unknown default branch")
	}
	devExists, err := s.GitHub.BranchExists(ctx, "dev")
	if err != nil {
		return fmt.Errorf("check dev branch: %w", err)
	}
	if devExists {
		return nil
	}
	current, err := s.Git.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	if current != "main" {
		return fmt.Errorf("remote dev is missing; run apply from main to create it safely (current branch: %s)", current)
	}
	if err := s.Git.EnsureDev(ctx); err != nil {
		return fmt.Errorf("create local dev branch: %w", err)
	}
	if err := s.Git.Push(ctx, "dev"); err != nil {
		return fmt.Errorf("push dev branch: %w", err)
	}
	return nil
}

func labels() []github.Label {
	return []github.Label{
		{Name: "bug", Color: "d73a4a", Description: "Something is not working"},
		{Name: "feature", Color: "a2eeef", Description: "A human-implemented feature"},
		{Name: "enhancement", Color: "84b6eb", Description: "Improvement to existing behavior"},
		{Name: "ai-analyzed", Color: "7057ff", Description: "Issue has a RepoFlow AI analysis"},
		{Name: "ai-working", Color: "fbca04", Description: "AI is currently analyzing or testing"},
		{Name: "ai-failed", Color: "b60205", Description: "AI assistance could not produce a safe fix"},
		{Name: "ai-rate-limited", Color: "5319e7", Description: "AI analysis was deferred because the author exceeded the rate limit"},
		{Name: "release", Color: "0e8a16", Description: "Release-related change"},
	}
}

func expectEqual(actual, expected string) error {
	if actual != expected {
		return fmt.Errorf("expected %s, got %s", expected, actual)
	}
	return nil
}

func expectMergeSettings(repo github.Repository) error {
	if !repo.AllowSquashMerge || !repo.AllowMergeCommit || repo.AllowRebaseMerge || repo.AllowAutoMerge {
		return errors.New("expected squash and merge-commit enabled, rebase and auto-merge disabled")
	}
	return nil
}

func localFile(root string, paths ...string) error {
	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "404") || strings.Contains(text, "not found")
}

func (s Service) String() string {
	return strings.TrimSpace(s.Root)
}
