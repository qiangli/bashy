---
id: 39b5d2028677
kind: bug
title: 'Windows: TestInboxWatchDeliversANewBoardPostWithoutHumanRelay and TestInboxBoundedWaitDoesNotFinishOnOwnMeetPost fail'
seq: 412
status: todo
priority: p1
labels:
    - windows
created: 2026-10-08T19:41:07.995859Z
sprint: 379
sprint_id: 908c2ac2-e7bc-55fe-86bd-d25046ac4684
sprint_title: 'bashy 1.0.0 feature list: bash + Bash# + Yoke'
---

B1 evidence lane 2026-10-08, noviwin1.local, candidate bashy ffc629b1 (GOWORK=off, go1.27.0): go test ./internal/agentos FAIL with exactly these two tests; macOS/Linux pass. Raw log: noviwin1 %USERPROFILE%\s379-claude\win-b1.log. Reproduce on noviwin1, find the Windows-only cause (timing, file watch, path), fix at root with a test. Blocks ci-green (bashy windows-latest).

Worker rules (Sprint 379 conductor, 2026-10-08): work in the submodule named above; commit and push INSIDE it to its default branch (public repos: main), then stop - the conductor bumps the umbrella pin. Every commit carries Sprint: #379 plus Story/Story-ID trailers for this story, with BASHY_AGENT set to your own binding. Reproduce first with a red unit test, fix at the root, never skip, quarantine or add to a known-failures list. This dev box (Dragon) is for builds and focused unit tests only. Windows runs go to noviwin1.local, Linux runs to the novidesign.local podman machine bashy or the repo CI, and macOS full suites to novidesign.local (all passwordless ssh; use the .local names). Gate before push: go build ./... and go vet on the module, plus the focused tests, with the exit code captured (never gate a push on a pipe). Report: the root cause, the commit SHA, the exact tests run and where, and any CI run URL.
