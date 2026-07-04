# Local Loom SDLC Control Plane

This is the preferred default for public repositories and subscription-backed
agents.

## Principle

Use GitHub only for what should be public or externally integrated:

- source control
- pull/push synchronization
- release or deployment handoff to external systems such as GitHub Pages,
  DigitalOcean, AWS, or other CI/CD providers

Keep SDLC orchestration local/private:

- issue intake mirror
- issue comments and conductor logs
- subscription-backed agent execution
- QA notes and human approval
- retry state and failure evidence

That split avoids putting a developer workstation's agent subscription, local
work logs, or broad write authority inside a public GitHub Actions runner.

## Happy Path

```text
user files request
        |
        v
local trigger selects work
        |
        v
bashy sdlc tick/delegate
        |
        v
conductor agent plans, edits, tests, commits
        |
        v
optional QA accepts or rejects
        |
        v
optional human approves rollout
        |
        v
bashy sdlc rollout
        |
        v
push source or deployment signal to GitHub/external target
        |
        v
bashy sdlc verify + resolve
```

On rejection or failure, the trigger calls `bashy sdlc tick` again with the same
request pointer plus the new evidence. The SDLC reference id remains the join key
for QA, approval, rollout, and resolution.

## Runtime Pieces

- `bashy loom serve`: local Gitea-based forge/control plane.
- `bashy schedule`: local periodic trigger for polling selected work.
- `bashy sdlc issue`: local issue/request record when the intake is not yet a
  real forge issue.
- `bashy sdlc tick`: one externally selected work item delegated to the
  conductor.
- `bashy sdlc watch`: progress monitor for background conductor runs.
- `bashy sdlc qa`: optional QA accept/reject gate.
- `bashy sdlc approve`: optional human approval gate.
- `bashy sdlc rollout`: delegates deployment rollout after approval.
- `bashy sdlc verify`: checks local or remote evidence.
- `bashy sdlc resolve`: marks the exact SDLC reference complete.

## Minimal Local Workflow

```sh
bashy loom serve --addr 127.0.0.1 --port 3000
git clone https://github.com/owner/repo.git ~/work/repo
cd ~/work/repo

bashy sdlc issue --text "Fix the broken link"
bashy sdlc tick \
  --issue-file .bashy/sdlc/issues/<printed-issue-file>.md \
  --intake-provider loom \
  --intake-repo owner/repo \
  --production github-pages \
  --background

bashy sdlc watch --follow
bashy sdlc runs
bashy sdlc qa RUN_ID --status accepted --note "local smoke passed"
bashy sdlc approve RUN_ID --note "approved for rollout"
bashy sdlc rollout RUN_ID --production github-pages --background
bashy sdlc verify --url https://example.com --present "expected text"
bashy sdlc resolve RUN_ID --status resolved --note "deployment verified"
```

## GitHub Boundary

The local conductor may push source changes to GitHub after the local work is
accepted. GitHub Actions may then deploy to external targets, or the local
rollout step may call the external target directly. In both cases, GitHub is not
the place where private issue discussion, conductor logs, or subscription-backed
agent execution state lives.

For the first GitHub Pages use case:

```text
local SDLC resolves source change -> push main -> GitHub Pages deploys -> SDLC verifies live URL
```

That is simpler and safer than a public GitHub-hosted workflow calling a local
agent subscription.

## Design Gaps — status (updated 2026-07-03)

Closed by the label-driven SDLC work (`coreutils/pkg/sdlc/`):

- **GitHub issue intake — DONE.** `--intake-provider github` now really fetches
  and selects the next eligible issue (`github_intake.go` `nextGitHubIssue` /
  `SelectNextIssue`), label-aware and priority-ordered, PR-skipping. Reserved
  label families in `labels.go` (`sdlc:*` lifecycle + `deploy:*` baton).
- **Issue selection in `bashy sdlc tick` — DONE.** `Prepare` auto-selects when no
  `--issue` is passed (`intake_wire.go`), gated by `sdlc:go`
  (`intake_require_initiate`, default true), claiming `sdlc:in-progress` so a
  concurrent tick can't double-pick. Empty queue = a clean `idle` no-op.
- **Durable trigger daemon — DONE.** `bashy sdlc service {start,status,stop,run}`
  (`service.go`) is the always-on loop, supervised by outpost as an opt-in
  `conf.BashyService` (`Command: ["sdlc","service"]`) with self-heal auto-install.
- **Deploy adapter / baton — DONE.** `PromoteByLabel` applies `deploy:<env>` (the
  trigger for the deploy GitHub Action — `github_write.go`), gated by the
  `prod_approval` policy (`promote.go` `RequiresApproval`); `RunDeployTarget`
  (`deploy_local.go`) makes `TargetConfig.{Command,Healthcheck,Rollback}`
  executable for the direct/local path. `sdlc resolve` closes the board
  (`SyncGitHubResolution`: clear claim → `sdlc:done` → close).

Still open:

- First-class GitHub↔Loom issue *mirroring* (copy public issues into private
  Loom state). The GitHub fetch + private-execution split exists; a two-way
  mirror is the remaining nicety.
- Richer deployment adapters (SSH, Kubernetes/Argo, AWS) beyond GitHub-Pages /
  the label-baton + local-command paths.
