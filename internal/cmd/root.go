package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/repoflow/gh-repoflow/internal/app"
	"github.com/repoflow/gh-repoflow/internal/command"
	"github.com/repoflow/gh-repoflow/internal/config"
)

func Execute(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage(out)
		return 0
	}
	if args[0] == "version" || args[0] == "--version" {
		fmt.Fprintln(out, "gh-repoflow v0.1.4")
		return 0
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(errOut, "repoflow: determine working directory: %v\n", err)
		return 1
	}
	runner := command.ExecRunner{Dir: root, Stderr: errOut}
	service := app.New(root, runner, out)
	var runErr error
	switch args[0] {
	case "init":
		runErr = runInit(ctx, service, args[1:], out, errOut)
	case "apply":
		runErr = runApply(ctx, service)
	case "check":
		runErr = runCheck(ctx, service)
	default:
		runErr = fmt.Errorf("unknown command %q", args[0])
	}
	if runErr != nil {
		fmt.Fprintf(errOut, "repoflow: %v\n", runErr)
		return 1
	}
	return 0
}

func runInit(ctx context.Context, service app.Service, args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(errOut)
	name := flags.String("name", "", "GitHub repository name (defaults to current directory)")
	visibility := flags.String("visibility", "", "repository visibility: public, private, or internal")
	public := flags.Bool("public", false, "create a public repository")
	private := flags.Bool("private", false, "create a private repository")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *public && *private {
		return errors.New("--public and --private cannot be used together")
	}
	selectedVisibility := *visibility
	if *public {
		selectedVisibility = "public"
	}
	if *private {
		selectedVisibility = "private"
	}
	if selectedVisibility != "" && selectedVisibility != "public" && selectedVisibility != "private" && selectedVisibility != "internal" {
		return fmt.Errorf("invalid visibility %q", selectedVisibility)
	}
	fmt.Fprintln(out, "Initializing RepoFlow...")
	return service.Bootstrap(ctx, app.InitOptions{Name: *name, Visibility: selectedVisibility})
}

func runApply(ctx context.Context, service app.Service) error {
	path := filepath.Join(service.Root, filepath.FromSlash(config.Path))
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load configuration: %w; run gh repoflow init first", err)
	}
	return service.Apply(ctx, cfg)
}

func runCheck(ctx context.Context, service app.Service) error {
	path := filepath.Join(service.Root, filepath.FromSlash(config.Path))
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load configuration: %w; run gh repoflow init first", err)
	}
	return service.Check(ctx, cfg)
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "RepoFlow - event-driven GitHub repository governance\n\n"+
		"Usage:\n"+
		"  gh repoflow version\n"+
		"  gh repoflow init [--name NAME] [--public|--private]\n"+
		"  gh repoflow apply\n"+
		"  gh repoflow check\n\n"+
		"Commands:\n"+
		"  init    Bootstrap git, GitHub, branches, templates, governance, and checks\n"+
		"  apply   Reconcile GitHub state with .github/repoflow.yml\n"+
		"  check   Inspect state without changing it")
}
