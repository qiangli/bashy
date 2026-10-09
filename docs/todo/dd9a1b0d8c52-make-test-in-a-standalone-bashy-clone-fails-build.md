---
id: dd9a1b0d8c52
kind: bug
title: 'make test in a standalone bashy clone fails: build-meet-spa.sh needs ../yoke'
seq: 411
status: done
priority: p0
labels:
    - release
created: 2026-10-08T19:34:56.919283Z
assignee: codex-gpt5.6-terra
sprint: 379
sprint_id: 908c2ac2-e7bc-55fe-86bd-d25046ac4684
sprint_title: 'bashy 1.0.0 feature list: bash + Bash# + Yoke'
closed: 2026-10-08T19:45:25.369418Z
closed_by: claude-opus5.5
---

Found by B1 evidence lane 2026-10-08 on novidesign.local, standalone clone of bashy ffc629b1, GOWORK=off: make test -> scripts/build-meet-spa.sh check -> 'meet SPA: missing ../yoke/pkg/meet/web package.json or pnpm-lock.yaml' -> make: *** [test-meet-spa-fresh] Error 1. Since Sprint 390 siblings are go.mod pins (docs/bashy-go-module-contract.md); a standalone clone has no ../yoke. Fix: resolve the yoke module directory from the go.mod pin (go mod download -json github.com/qiangli/yoke -> Dir, or scripts/resolve-pinned-siblings.sh) when ../yoke is absent; keep ../yoke for the umbrella. Red/green: run make test-meet-spa-fresh (or the check) in a standalone clone. Blocks B1 (make test is a release gate).

Worker rules (Sprint 379 conductor, 2026-10-08): work in the submodule named above; commit and push INSIDE it to its default branch (public repos: main), then stop - the conductor bumps the umbrella pin. Every commit carries Sprint: #379 plus Story/Story-ID trailers for this story, with BASHY_AGENT set to your own binding. Reproduce first with a red unit test, fix at the root, never skip, quarantine or add to a known-failures list. This dev box (Dragon) is for builds and focused unit tests only. Windows runs go to noviwin1.local, Linux runs to the novidesign.local podman machine bashy or the repo CI, and macOS full suites to novidesign.local (all passwordless ssh; use the .local names). Gate before push: go build ./... and go vet on the module, plus the focused tests, with the exit code captured (never gate a push on a pipe). Report: the root cause, the commit SHA, the exact tests run and where, and any CI run URL.
