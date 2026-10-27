---
id: ccd61831dddc
kind: bug
title: S514.R1 bashy test workflow green on macOS, Linux and Windows
seq: 2728
status: todo
priority: p0
labels:
    - release
    - ci
created: 2026-10-27T07:53:02.492313Z
sprint: 514
sprint_id: fadd1a46-0368-50f1-a65d-4ed147b17629
sprint_title: 'invitation email + scoped satellites (KISS/MVP: scope + expiry on the one-time pairing code; no outpost change)'
---

Release bar (runbook #3 kb:release-bashy): bashy test workflow must be GREEN on all three OS on the candidate commit; never tag around a red lane. Red on main 23e00d4c (run 40468480618 / 40466576387).
All OS: TestCommandDocSourceNames (internal/agentos/commanddocs_test.go:30) — Names() now includes 'transcribe' (S503 doc); update the expected list.
Windows only: TestAdoptionThroughWireExecHonorsHintsOff (on/off), TestAdoptionProjectionsMatchSource, TestShimAgentEnvMarkers, TestCommonOptionsVocabularyReport, TestHelpExcludedToolsAreUnsupported, TestInspectContextRedirectedStdio, then 'panic: test timed out after 5m0s' in the same lane. Root-cause each (Windows paths/.exe/env case-insensitivity/stdio); fix the code or the test expectation, whichever is wrong; no skips without a recorded reason.
Local macOS also fails TestRunSignaledIsNonLossy (run_test.go:51) — check whether CI runs it; fix if it is a real bug.
Gate: go test -short of the failing packages on macOS (Dragon) + Windows (otter via outpost ssh, or windows-latest CI); then the bashy test workflow green on all 3 OS for the pushed commit.
