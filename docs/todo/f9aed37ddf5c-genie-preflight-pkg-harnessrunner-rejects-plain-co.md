---
id: f9aed37ddf5c
kind: bug
title: genie preflight (pkg/harnessrunner) rejects plain compound workspace commands (pwd; echo x > f; cat f)
seq: 413
status: done
priority: p2
labels:
    - genie
created: 2026-10-08T20:38:36.431675Z
assignee: codex-gpt5.6-terra
sprint: 379
sprint_id: 908c2ac2-e7bc-55fe-86bd-d25046ac4684
sprint_title: 'bashy 1.0.0 feature list: bash + Bash# + Yoke'
closed: 2026-10-08T20:45:09.037204Z
closed_by: claude-opus5.5
---

Moved from yoke story 84d40c38 (wrong repo; codex-gpt5.6-terra triage 2026-10-08): genie's command preflight is bashy pkg/harnessrunner (compileScript -> walkCompile in refine_funcs.go). Repro: bashy genie in a scratch git repo asked to run exactly 'pwd; echo steward-ok > f.txt; cat f.txt' answers 'The command was rejected by the reviewer'; the single write passes. Determine whether the refusal is the ';' list, the '>' redirect, or cat's effect refinement - write the red unit test against CompileIntent first. Fix (KISS): a ;-joined list of simple commands is classified by its parts, and a redirect to a path inside the writable root is a workspace write. Red/green: the compound is allowed; the same compound writing outside the workspace (> /etc/x or ../x) is still refused. Do not touch yoke pkg/agentlaunch or pkg/fleet identity code (Sprint 321).

Worker rules (Sprint 379 conductor, 2026-10-08): work in the submodule named above; commit and push INSIDE it to its default branch (public repos: main), then stop - the conductor bumps the umbrella pin. Every commit carries Sprint: #379 plus Story/Story-ID trailers for this story, with BASHY_AGENT set to your own binding. Reproduce first with a red unit test, fix at the root, never skip, quarantine or add to a known-failures list. This dev box (Dragon) is for builds and focused unit tests only. Windows runs go to noviwin1.local, Linux runs to the novidesign.local podman machine bashy or the repo CI, and macOS full suites to novidesign.local (all passwordless ssh; use the .local names). Gate before push: go build ./... and go vet on the module, plus the focused tests, with the exit code captured (never gate a push on a pipe). Report: the root cause, the commit SHA, the exact tests run and where, and any CI run URL.
