# gh-repoflow

gh repoflow is a GitHub CLI extension for an event-driven repository workflow.
It standardizes repository settings, main/dev branches, rulesets, labels,
templates, CI, AI automation boundaries, and releases without introducing
calendar cycles or automatic merging.

## Install

Build and install the extension locally:

~~~powershell
go build -o gh-repoflow.exe .
gh extension install .
~~~

Or install from a GitHub fork:

~~~bash
gh extension install wkzmagician/gh-repoflow
~~~

The host must have git, gh, and an authenticated GitHub CLI session:

~~~bash
gh auth login
~~~

## Usage

From a project directory:

~~~bash
gh repoflow init
gh repoflow apply
gh repoflow check
~~~

init bootstraps the local repository, creates the GitHub repository when
needed, creates and pushes main and dev, generates .github/repoflow.yml and
the standard workflow files, then runs apply and check.

apply reconciles GitHub settings, branches, rulesets, collaborators, and
labels with .github/repoflow.yml. It is safe to run repeatedly and never
deletes unknown rulesets or force-pushes branches.

check is read-only and reports each failed expectation with the actual
reason. It never changes local or GitHub state.

## Default flow

~~~text
feature/* ─┐
fix/*     ─┼── PR ──> dev ── release PR ──> main ──> human-triggered release
ai/*      ─┘
~~~

Direct pushes, force pushes, and deletion are blocked on main and dev.
Feature work remains human-owned. AI workflows are deliberately provider
integration boundaries: they may assist with bugs and CI failures, but may not
merge, release, alter secrets/rulesets, or implement features.

## Configuration

.github/repoflow.yml is the source of truth:

~~~yaml
repository:
  visibility: public

branches:
  main:
    approvals: 2
  dev:
    approvals: 1

developers: []

ci:
  enabled: true

ai:
  enabled: true
  provider:
    type: openai-compatible
    base_url: https://openrouter.ai/api/v1
    model: xxx
    api_key: secret:REPOFLOW_AI_API_KEY
  analysis:
    issues: true
  policy:
    auto_fix_bugs: true
    min_confidence: 0.85
  ci_debug: true

release:
  enabled: true
~~~

RepoFlow intentionally rejects timezone, schedule, cycle, week, and weekday
fields. Human review and release timing are outside its scope.

AI analysis runs for every opened, edited, or reopened Issue. The model only
returns a structured proposal; RepoFlow policy decides whether the result is
analysis_only, needs_human, or auto_fix. Configure the API key without putting
it in this file:

~~~bash
gh secret set REPOFLOW_AI_API_KEY
gh variable set REPOFLOW_AI_BASE_URL --body https://openrouter.ai/api/v1
gh variable set REPOFLOW_AI_MODEL --body <model>
~~~

AI analysis has only contents-read and issues-write permissions. Code mutation
is isolated in a separate workflow and can create an ai/* branch and a PR to
dev, but cannot merge, publish, modify governance, secrets, or workflows.

## Development

~~~bash
go fmt ./...
go test ./...
go vet ./...
go build .
~~~
