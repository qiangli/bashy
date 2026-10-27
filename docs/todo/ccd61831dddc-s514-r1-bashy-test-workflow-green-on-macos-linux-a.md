---
id: ccd61831dddc
kind: bug
title: S514.R1 bashy test workflow green on macOS, Linux and Windows
seq: 2728
status: done
priority: p0
labels:
    - release
    - ci
created: 2026-10-27T07:53:02.492313Z
weave: 289
assignee: claude-sonnet5.5
sprint: 514
sprint_id: fadd1a46-0368-50f1-a65d-4ed147b17629
sprint_title: 'invitation email + scoped satellites (KISS/MVP: scope + expiry on the one-time pairing code; no outpost change)'
closed: 2026-10-27T09:57:59.708697Z
resolution: fixed
closed_by: claude-sonnet5.5-c
---

Release bar (runbook #3 kb:release-bashy): bashy test workflow must be GREEN on all three OS on the candidate commit; never tag around a red lane. Red on main 23e00d4c (run 40468480618 / 40466576387).
All OS: TestCommandDocSourceNames (internal/agentos/commanddocs_test.go:30) — Names() now includes 'transcribe' (S503 doc); update the expected list.
Windows only: TestAdoptionThroughWireExecHonorsHintsOff (on/off), TestAdoptionProjectionsMatchSource, TestShimAgentEnvMarkers, TestCommonOptionsVocabularyReport, TestHelpExcludedToolsAreUnsupported, TestInspectContextRedirectedStdio, then 'panic: test timed out after 5m0s' in the same lane. Root-cause each (Windows paths/.exe/env case-insensitivity/stdio); fix the code or the test expectation, whichever is wrong; no skips without a recorded reason.
Local macOS also fails TestRunSignaledIsNonLossy (run_test.go:51) — check whether CI runs it; fix if it is a real bug.
Gate: go test -short of the failing packages on macOS (Dragon) + Windows (otter via outpost ssh, or windows-latest CI); then the bashy test workflow green on all 3 OS for the pushed commit.

REWORK (s514-manager, from windows-latest job 122087507313 on a33856ef; macOS/Linux now green):
1. TestAdoptionThroughWireExecHonorsHintsOff (on/off): "TempDir RemoveAll cleanup: unlinkat ...\exec\<date>\ep-<id>.jsonl: The process cannot access the file because it is being used by another process." The exec episode recorder keeps the .jsonl open after the command finishes. That is a real Windows bug (an open handle locks the file): close the episode file when the record is written (or before the process exits the wire exec), not only at process exit.
2. TestInspectContextRedirectedStdio: "dispatchContext --json hung with redirected stdout; exceeded 5s deadline" (context_test.go:205).
3. Then "panic: test timed out after 5m0s" while TestDispatchDoctorJSONIncludesSelfChecks had run 3m35s. Likely the same root cause as 2 (a probe that blocks on Windows when stdout is not a console, e.g. a console/TTY query, a network or WSL probe without a deadline). Find the blocking call and bound it; do not raise test timeouts.
Reproduce on the Windows test host: the conductor gives its address on the sprint card; the dev box reaches it with `$HOME/bin/outpost ssh <user>@<host>:2222 '<cmd>'` (key auth, no password). Build a Windows test binary with `GOOS=windows go test -c ./internal/agentos -o agentos.test.exe`, copy it with `$HOME/bin/outpost scp -P 2222`, run `agentos.test.exe -test.run '<name>' -test.v` there. Gate: those tests pass on the Windows host, plus the bashy test workflow green on windows-latest. No hostnames in code, tests or docs.

REWORK 3 (s514-manager, windows-latest run 40481199887 on a666ad8e; macOS green; the 5m timeout is gone, the package now finishes in 246s and shows these previously masked Windows failures). Full CI log: ~/.bashy/s514-windows-ci-a666ad8e.log. Windows test host: read `user@host:port` from ~/.bashy/s514-windows-test-host and reach it with `$HOME/bin/outpost ssh <that> '<cmd>'` / `$HOME/bin/outpost scp -P <port>` (key auth; the shell there is bashy). Build `GOOS=windows go test -c ./internal/agentos`, copy the repo tree (`git archive --format=zip HEAD`, unpack with `C:/Windows/System32/tar.exe -xf`), run the test exe from internal/agentos with -test.run. Note: a full-package run on that host also shows host-state failures (git, inbox, genie bindings) that CI does not; the gate is CI.
1. TestInstallAgentMCPMalformedPrefsFailsClosed: "install-agent: codex: fleet: invalid BASHY_PREFS: unexpected end of JSON input" then "config written despite malformed prefs" (installagent_prefs_mcp_test.go:88). Fail-closed must hold on Windows: find why a config is written there.
2. TestManagedOwnerWatchDrainsOverflowAndPreservesOwnership / ...SecondWatchStillRefused / ...LeaveKeepsForemanCard: "watch inside the owning managed session refused: inbox: register watcher ...: room ... is already live (pid N)" (inbox_managed_drain_test.go:74/124/177). The same-process/owner recognition differs on Windows.
3. TestMailboxDirFollowsBashyHomeThenIsolatedHome: "BASHY_HOME inbox not created 0700" (mailbox_home_test.go:77): Windows has no unix mode bits; assert the platform-appropriate result.
4. TestReaderPrefsBudget/redaction_and_canonicalization_hold_under_budget: read the log for the exact assertion.
Fix product bugs in the product, wrong expectations in the test; no skips without a reason recorded in the test. Gate: these pass on the Windows host, then windows-latest green.
