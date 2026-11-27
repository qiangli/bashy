---
id: 95f11ca25e17
kind: bug
title: discover tests leak the serve fixture's python server after the test ends
seq: 3327
status: todo
priority: p3
created: 2026-11-27T12:59:11.408834Z
---

Found 2026-11-27 on the dev box: two `discover-serve.*.py` python servers started 2026-11-22 by examples/shaping/discover tests (TestMatrixAgainstFixture, TestAgentLegHeadersFromLog) were still running, re-parented to PID 1, serving a weave workspace (umbrella-f8ff73b3/workspaces/issue-160) that had since been deleted. Stopped by hand.

Where: discover_test.go `startFixture` starts `bashy --bashsharp discover.bsh serve` with exec.CommandContext and its Cleanup cancels the context, which kills only the bashy process. discover.bsh `cmd_serve` ends with `exec "$(python_bin)" "$srv" ...`. The orphan's PPID 1 says python ran as a CHILD of bashy (the in-process `exec` did not replace the process image here), so killing bashy left it behind. Root cause still to confirm: whether bashy's `exec` builtin forks in this path, or the leak comes from elsewhere.

Fix direction: start the fixture in its own process group (SysProcAttr Setpgid) and kill the group in Cleanup, or have `serve` trap its own termination and stop the server. Add a check that no server outlives the test. The temp `discover-serve.XXXXXX.py` script in $TMPDIR is also never removed after a real run.
