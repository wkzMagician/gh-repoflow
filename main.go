package main

import (
	"context"
	"os"

	"github.com/repoflow/gh-repoflow/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
