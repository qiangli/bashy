---
id: f99a22228b41
kind: bug
title: 'weave gc: reclaim state roots that have no queue.json'
seq: 3326
status: done
priority: p3
created: 2026-11-27T12:57:29.845875Z
closed: 2026-11-27T13:11:06.391298Z
---

Found 2026-11-27 cleaning ~/.bashy/weave by hand: `bashy weave gc` reported "nothing to reclaim" while 19 state roots were leftovers (work-* from deleted /tmp shaping demos, gate-*, clone-umbrella, s684-final-umbrella, umbrella-s698-verified, bashy660, s690-parity). Root cause: `weaveGCQueueDirs` (yoke pkg/weave/weave_gc.go) only walked roots that have queue.json; these held only memory.jsonl, locks, session.json and feed cursors, so gc never saw them.

Fixed (adhoc, yoke main 229e5699, pinned in bashy by this commit): a root without queue.json is reclaimed when every entry is weave bookkeeping or a file-less directory and nothing changed for 24 hours. Logs with content, workspaces, unknown files (e.g. the `tools` root) or hidden directories (`.go-build-cache`) keep it. Test: TestWeaveGCStatelessRoots and TestWeaveGCReportThenApply PASS on the gate host (puppy VM, go1.27.4, 2026-11-27).

Not in this fix (separate): two discover-serve python fixture servers (bashy examples/shaping/discover tests TestMatrixAgainstFixture, TestAgentLegHeadersFromLog) outlived their test run since 2026-11-22, orphaned to PID 1 and serving a deleted weave workspace. Ships to bashy with the next yoke pin bump.
