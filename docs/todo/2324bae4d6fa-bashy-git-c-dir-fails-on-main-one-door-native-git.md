---
id: 2324bae4d6fa
kind: bug
title: 'bashy git -C DIR fails on main: one-door native git refuses -C'
seq: 418
status: todo
priority: p0
labels:
    - git
created: 2026-10-09T04:16:45.826933Z
sprint: 404
sprint_id: d0936923-9388-5532-b329-e23e58422343
sprint_title: bashy git -C regression and canonical reinstall
---

Found by the Sprint 321 conductor 2026-10-09 after env make install of bashy main 7be3316: 'bashy git -C DIR log -1' fails with "unknown shorthand flag: 'C' in -C"; the installed fe23b23 forwards it to real git and works; 'bashy git --external -C DIR ...' also works. Agent shells wrap git as a function calling 'bashy git', so every git -C in every agent breaks once main is installed. Introduced by the Sprint 252 git one-door (6c39ef39 and later). The canonical ~/.local/bin/bashy was rolled back to fe23b23. Fix: the one-door must accept -C (and other global options it does not implement natively) by delegating to real git, per the bashy-git-is-real-git rule; add a red test for git -C before the fix. Release blocker for the next install.

BRIEF (Sprint 404, conductor claude-opus5.5): baseline bashy origin/main. The one-door lives in internal/agentos/git.go (gitCmd: native engine by default, --external allows host git fallback, cobra parses argv with UnknownFlags whitelisted, yet a leading -C still fails with "unknown shorthand flag: 'C' in -C"). KISS fix: git's global options BEFORE the verb (-C DIR, -c k=v, --git-dir, --work-tree, --no-pager) must be accepted and applied exactly as real git would (e.g. -C = run the verb in DIR) for both the native path and the --external fallback. Red test first: `bashy git -C <repo> log -1 --format=%h` and `bashy git -C <repo> status --short` from another cwd. Do NOT change the native-by-default policy. Then a smoke (report table, not fixes) of these through `bashy git ...` in a temp repo with a temp bare remote: status, log, diff, add, commit, fetch, pull --ff-only, push, rev-parse, ls-remote, worktree add/remove, cherry-pick, branch, show, merge-base, each with and without -C. File any other failure as a separate p1 todo in bashy, do not fix it here. Commit on branch agent/s404-git-c from origin/main with BASHY_AGENT set to your injected identity and trailers as the final paragraph: Sprint: #404 / Story: #418 / Story-ID: 2324bae4d6fa. Do not push main. Short focused tests only (GOWORK=off go test -short -count=1 -timeout 180s ./internal/agentos -run Git). Hard limit 45 minutes. Final message: branch, SHA, red/green test names, smoke table.
