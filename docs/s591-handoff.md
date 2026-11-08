# Sprint 591 — handoff (2026-11-08)

Manager so far: `claude-opus5.5`. Plan: [plan-s591-readme-agent-bootstrap.md](plan-s591-readme-agent-bootstrap.md).

## Delivered

| What | Where |
|---|---|
| README = human hook + agent skill (one consent gate a / a+b), runnable as `bashy dag README.md` | bashy `60b38856` |
| Reference moved to `docs/install.md`, `docs/usage.md`, `docs/overview.md` | bashy `60b38856` |
| Raw-README pointer for agents (top of README and install.md) | bashy `5b34148b` |
| GitHub About: prose description (bash, Bash#, Yoke, software shaping), website, topics | repo settings |
| Muse MCP entry printed in muse's schema (`integration.mcp.entry` / `.root`) | yoke `758c6859` (not yet pinned into bashy) |
| Installer HEAD probes return 302 like GET | hosted portal repo, committed, **not deployed** |

## Cold start from the real URL (fresh non-root container per agent)

| Agent | Consent ask | Install + connect | New session |
|---|---|---|---|
| claude | yes, once | pass | skill, MCP, bashy shell: PASS |
| codex | no (headless exec) | pass after `5b34148b` | PASS |
| agy | yes, once | pass | PASS |
| muse | yes, once | pass (tarball fallback; MCP entry hand-fixed) | skill + MCP ok, shell `/bin/sh`: PARTIAL |

Option (b), the outpost boot service, is untested (no systemd in containers); per operator, assume it works and file bugs if it fails.

## Open stories — suggested fleet split

Independent, small, one repo each, so they can run in parallel in weave workspaces.

| Story | Repo | Suggested worker | Note |
|---|---|---|---|
| `b83e76fc7d4b` MCP registered as bare `bashy` | yoke `pkg/skills/integrate.go` (`mcpServerEntry`) | codex | register the resolved absolute path; it worked around this itself |
| `4c99628d6704` `skill export --dry-run` only gates MCP | yoke `pkg/skills/cli.go` | claude (sonnet) | dry-run must preview the whole export |
| `e97a9a5d792f` muse shell not routed through bashy | bashy `internal/agentos/installagent.go` | muse | first find muse's shell knob; it runs `/bin/sh` absolutely |
| `51c7c587d8b8` installer defaults to `--service` | outpost `scripts/install.{sh,ps1}` | agy | pick: default `--no-service`, or document the start |
| `a79c27e8286a` `outpost service install` masks sleep (sudo) | outpost | agy | make the power step opt-in; say it in `--help` |
| `a5f4b13ae7f8` viewer `register.sh` needs `GOWORK=off` | bashy `examples/desk/viewer` | glm | one-line fix |
| `2897cf51117c` promoted release reports `-dev` | bashy release train | manager | touches promotion; do with the next release |
| `8a9f0dede7c6` installer HEAD 404 | hosted portal | operator | fix is committed; needs a production deploy decision (its branch is far ahead of the deployed build) |
| yoke pin bump into bashy | bashy `go.mod` + sibling pins | manager | ships `758c6859`; needs the golden refresh noted in the release notes |

## State left behind

- The podman VM on the dev box is **stopped, not removed**: its volumes `muse-auth` and `agy-auth` hold the operator's muse and agy sign-ins for later cold starts. `bashy oci machine rm -f <name>` discards them.
- Raw cold-start logs stay in the session scratchpad, not in git.
- The bashy viewer runs on loopback `:18096` with the About drafts (book `github-about`).

## Lessons

- Agents read a GitHub URL through a summarizer; give them the raw URL early.
- Probe with GET, not HEAD, before calling an endpoint down.
- Goldens that record closure names depend on the Go toolchain; regenerate with the CI version (go1.27.2), never `GOTOOLCHAIN=auto` from a `go.work` that says 1.26.
