package github

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type topicRunner struct {
	topicsResponse string
	putArgs        []string
	putInput       []byte
}

func (r *topicRunner) Run(_ context.Context, _ string, args ...string) (string, error) {
	if strings.Contains(strings.Join(args, " "), "GET") {
		if r.topicsResponse != "" {
			return r.topicsResponse, nil
		}
		return `{"names":["existing"]}`, nil
	}
	return "", nil
}

func (r *topicRunner) RunInput(_ context.Context, input []byte, _ string, args ...string) (string, error) {
	r.putArgs = append([]string(nil), args...)
	r.putInput = append([]byte(nil), input...)
	return "", nil
}

func TestEnsureTopicPreservesExistingTopics(t *testing.T) {
	runner := &topicRunner{}
	client := Client{Runner: runner, Repo: "owner/project"}

	if err := client.EnsureTopic(context.Background(), "Dartloom"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(runner.putArgs, " "), "/topics") {
		t.Fatalf("expected topics PUT, got %v", runner.putArgs)
	}

	var payload struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(runner.putInput, &payload); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(payload.Names, ","), "dartloom,existing"; got != want {
		t.Fatalf("topics = %q, want %q", got, want)
	}
}

func TestEnsureTopicIsIdempotent(t *testing.T) {
	runner := &topicRunner{topicsResponse: `{"names":["dartloom"]}`}
	client := Client{Runner: runner, Repo: "owner/project"}

	if err := client.EnsureTopic(context.Background(), "DARTLOOM"); err != nil {
		t.Fatal(err)
	}
	if len(runner.putArgs) != 0 {
		t.Fatalf("empty topic should not write topics: %v", runner.putArgs)
	}
}
