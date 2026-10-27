---
id: 9b588fdda9e0
kind: bug
title: 'S514.R5 TestCommonOptionsVocabularyReport flakes on CI: bin-managed ''why'' (witr) help depends on a download'
seq: 2731
status: done
priority: p1
labels:
    - release
    - ci
created: 2026-10-27T15:13:18.038284Z
weave: 292
assignee: muse-conductor
sprint: 514
sprint_id: fadd1a46-0368-50f1-a65d-4ed147b17629
sprint_title: 'invitation email + scoped satellites (KISS/MVP: scope + expiry on the one-time pairing code; no outpost change)'
closed: 2026-10-27T15:26:01.972274Z
resolution: fixed
closed_by: muse-conductor-f
---

Seen on macos-latest (run 40501474982, a74f8590) and windows-latest (run 40526415459, eb6a6c33); passes locally and on rerun. Failure: 'why: customGrammarTools=false but 0 flags parsed from --help' and the why rows of testdata/common_options_matrix.txt differ (why --json supported in the golden). Cause (to confirm): 'why' is the bin-managed external witr (yoke pkg/binmgr pins); toolHelp (internal/agentos/common_options_test.go:200) runs '--help' in a child with a 5s timeout, so on a cold CI cache the help depends on a GitHub download (slow, rate-limited) and comes back without flags. Make the audit deterministic without network: e.g. treat bin-managed externals whose binary is not cached as 'help unavailable -> skip with reason' (or read the pinned help the binmgr record carries), never a silent 0-flag help; keep the golden stable. No new skips without a reason in the test. Gate: go test -run TestCommonOptionsVocabularyReport with an empty binmgr cache (temp BASHY_HOME) passes deterministically; CI 3 OS green.
