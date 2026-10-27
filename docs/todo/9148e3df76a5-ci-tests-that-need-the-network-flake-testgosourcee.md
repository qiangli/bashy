---
id: 9148e3df76a5
kind: bug
title: 'CI tests that need the network flake: TestGoSourceExecutableEnvironment fetches go.dev release index (macos-latest timeout, run 40530939179); make them network-independent (use the installed SDK / local cache) like S514.R5'
seq: 2732
status: assigned
priority: p2
labels:
    - ci
created: 2026-10-27T15:42:37.481156Z
weave: 294
assignee: muse-conductor
sprint: 515
sprint_id: e82160ea-95db-5f2a-b69e-b2d784b2996b
sprint_title: 'satellite loom: private forge + CI + groups (KISS/MVP, reuse loom/act_runner/scopes)'
---

Bug: CI tests that need the network flake. TestGoSourceExecutableEnvironment fetches the go.dev release index and timed out on macos-latest (run 40530939179). Make it, and any sibling test that reaches the network the same way, network-independent: use the installed Go SDK / local cache (same approach as S514.R5). Do not skip the test; make the network path unreachable in tests (inject the index/source).
Red/green: the test passes with networking disabled (e.g. a bogus proxy / blocked resolver env) and still checks the same behaviour.
Gate: focused `go test -run TestGoSourceExecutableEnvironment` + the package containing it. Never exec the bashy self binary from tests.
Commit trailers: Sprint: 515 / Story: 9148e3df ci-network-independent / Story-ID: 9148e3df76a5.
