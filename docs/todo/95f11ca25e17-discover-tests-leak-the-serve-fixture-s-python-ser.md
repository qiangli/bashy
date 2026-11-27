---
id: 95f11ca25e17
kind: bug
title: discover tests leak the serve fixture's python server after the test ends
seq: 3327
status: done
priority: p3
created: 2026-11-27T12:59:11.408834Z
closed: 2026-11-27T13:18:02.114427Z
---

Found 2026-11-27 on the dev box: two `discover-serve.*.py` python servers started 2026-11-22 by examples/shaping/discover tests (TestMatrixAgainstFixture, TestAgentLegHeadersFromLog) were still running, re-parented to PID 1, serving a weave workspace (umbrella-f8ff73b3/workspaces/issue-160) that had since been deleted. Stopped by hand.

Where: discover_test.go `startFixture` starts `bashy --bashsharp discover.bsh serve` with exec.CommandContext and its Cleanup cancels the context, which kills only the bashy process. discover.bsh `cmd_serve` ends with `exec "$(python_bin)" "$srv" ...`. The orphan's PPID 1 says python ran as a CHILD of bashy (the in-process `exec` did not replace the process image here), so killing bashy left it behind. Root cause still to confirm: whether bashy's `exec` builtin forks in this path, or the leak comes from elsewhere.

Fix direction: start the fixture in its own process group (SysProcAttr Setpgid) and kill the group in Cleanup, or have `serve` trap its own termination and stop the server. Add a check that no server outlives the test. The temp `discover-serve.XXXXXX.py` script in $TMPDIR is also never removed after a real run.

## Resolution (2026-11-27, adhoc)

Root cause: `startFixture` cancelled a context that SIGKILLs only bashy. `serve` ends in `exec python3 …`, and when the interpreter cannot replace its image (sh interp `execReplace` falls back to a child when cwd or stdio do not match the process), the server runs as a child of bashy, so killing bashy orphaned it (PPID 1). Test defect, not product.

Fix: bashy 5f612099 runs the fixture in its own process group and kills the group in cleanup; `TestFixtureServerStopsWithTest` checks the port stops answering. Evidence, puppy VM container (golang:1.27, bashy v1.0.0-rc.41): on main a63b7c4 the two tests PASS and leave 2 `discover-serve` servers (reproduces the incident); on 5f612099 all three tests PASS and 0 servers remain.

Temp script (fixed 2026-11-27, bashy 031c3afc): the generated server deletes its own `discover-serve.XXXXXX.py` once loaded; the fixture runs with a private TMPDIR and `TestServeRemovesItsScript` checks none remains. Puppy VM container: all five discover tests PASS, 0 servers left.
