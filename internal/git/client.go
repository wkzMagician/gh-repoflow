package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/repoflow/gh-repoflow/internal/command"
)

type Client struct {
	Runner command.Runner
}

func (c Client) Version(ctx context.Context) error {
	_, err := c.Runner.Run(ctx, "git", "--version")
	return err
}

func (c Client) IsRepository(ctx context.Context) bool {
	_, err := c.Runner.Run(ctx, "git", "rev-parse", "--show-toplevel")
	return err == nil
}

func (c Client) HasCommits(ctx context.Context) bool {
	_, err := c.Runner.Run(ctx, "git", "rev-parse", "--verify", "HEAD")
	return err == nil
}

func (c Client) HasUncommittedChanges(ctx context.Context) (bool, error) {
	out, err := c.Runner.Run(ctx, "git", "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func (c Client) Init(ctx context.Context) error {
	_, err := c.Runner.Run(ctx, "git", "init")
	return err
}

func (c Client) CurrentBranch(ctx context.Context) (string, error) {
	out, err := c.Runner.Run(ctx, "git", "branch", "--show-current")
	return strings.TrimSpace(out), err
}

func (c Client) EnsureMain(ctx context.Context) error {
	branch, err := c.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	if branch == "main" {
		return nil
	}
	if branch == "" {
		return fmt.Errorf("repository is in detached HEAD; cannot initialize main safely")
	}
	if c.HasBranch(ctx, "main") {
		return fmt.Errorf("branch main already exists while current branch is %q; refusing to rename or overwrite it", branch)
	}
	_, err = c.Runner.Run(ctx, "git", "branch", "-M", "main")
	return err
}

func (c Client) HasBranch(ctx context.Context, name string) bool {
	out, err := c.Runner.Run(ctx, "git", "branch", "--list", name)
	return err == nil && strings.TrimSpace(out) != ""
}

func (c Client) EnsureDev(ctx context.Context) error {
	if c.HasBranch(ctx, "dev") {
		return nil
	}
	_, err := c.Runner.Run(ctx, "git", "branch", "dev")
	return err
}

func (c Client) Checkout(ctx context.Context, branch string) error {
	_, err := c.Runner.Run(ctx, "git", "checkout", branch)
	return err
}

func (c Client) AddAll(ctx context.Context) error {
	_, err := c.Runner.Run(ctx, "git", "add", "--all")
	return err
}

func (c Client) HasStagedChanges(ctx context.Context) (bool, error) {
	_, err := c.Runner.Run(ctx, "git", "diff", "--cached", "--quiet")
	if err == nil {
		return false, nil
	}
	// diff --cached --quiet exits 1 when changes exist. A command failure is
	// otherwise indistinguishable through the small Runner interface, so probe
	// the index with diff --cached as a second signal.
	out, probeErr := c.Runner.Run(ctx, "git", "diff", "--cached", "--name-only")
	if probeErr != nil {
		return false, probeErr
	}
	return strings.TrimSpace(out) != "", nil
}

func (c Client) Commit(ctx context.Context, message string) error {
	_, err := c.Runner.Run(ctx, "git", "commit", "-m", message)
	return err
}

func (c Client) HasOrigin(ctx context.Context) bool {
	_, err := c.Runner.Run(ctx, "git", "remote", "get-url", "origin")
	return err == nil
}

func (c Client) AddOrigin(ctx context.Context, url string) error {
	_, err := c.Runner.Run(ctx, "git", "remote", "add", "origin", url)
	return err
}

func (c Client) Push(ctx context.Context, branch string) error {
	_, err := c.Runner.Run(ctx, "git", "push", "-u", "origin", branch)
	return err
}
