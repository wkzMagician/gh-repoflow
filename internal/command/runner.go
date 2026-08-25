package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Runner is the small boundary around git/gh used by the application. Keeping
// it behind an interface makes the governance layer deterministic to test.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
	RunInput(ctx context.Context, input []byte, name string, args ...string) (string, error)
}

type ExecRunner struct {
	Dir    string
	Stdout io.Writer
	Stderr io.Writer
}

func (r ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	return r.run(ctx, nil, name, args...)
}

func (r ExecRunner) RunInput(ctx context.Context, input []byte, name string, args ...string) (string, error) {
	return r.run(ctx, input, name, args...)
}

func (r ExecRunner) run(ctx context.Context, input []byte, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if r.Dir != "" {
		cmd.Dir = r.Dir
	}
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), message)
	}
	if r.Stdout != nil && stdout.Len() > 0 {
		_, _ = io.Copy(r.Stdout, bytes.NewReader(stdout.Bytes()))
	}
	return stdout.String(), nil
}
