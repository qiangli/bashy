---
id: 9f947a58cd22
kind: bug
title: bashy runs its built-in cygpath while type reports a PATH cygpath (lookup mismatch)
seq: 3161
status: todo
priority: p1
labels:
    - shell
    - posix
    - windows
created: 2026-11-07T08:20:36.990109Z
---

Found 2026-11-07 on a rootful Linux gate machine; it is the cause of examples/outpost-profile TestWindowsOutPathConversion, which fails with network too.
Repro on Linux under bashy, with OS=Windows_NT and a fake cygpath first on PATH (/tmp/fb/cygpath):
  bare     cygpath -m /tmp/x/a                -> /tmp/x/a     (bashy built-in ran)
  explicit /tmp/fb/cygpath -m /tmp/x/a        -> C:/tmp/x/a
  via sh   /bin/sh -c 'cygpath -m /tmp/x/a'   -> C:/tmp/x/a
  type -a cygpath reports only /tmp/fb/cygpath
GNU Bash runs the PATH file (C:/tmp/x/agent.json in the test).
Expected: command execution resolves exactly as type/command -v report. A userland built-in must not shadow a PATH executable unless the documented lookup order says so, and then type must report the built-in. Investigate the command-lookup order for userland/coreutils-provided commands vs PATH in the sh engine / bashy dispatch.
Policy: the shell interpreter and coreutils are at the frozen reviewed baseline. Fix as a small, baseline-proven C commit with a regression test after operator OK; check the impact on the 86/86 and POSIX gates.
