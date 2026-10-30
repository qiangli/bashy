---
id: 69ec2df57b58
kind: bug
title: 'Windows: env-prefixed command in a pipeline fails lookup (''GOWORK=off go version | cat'' -> go: command not found)'
seq: 2964
status: todo
priority: p1
labels:
    - windows
    - shell
created: 2026-10-30T20:47:17.129806Z
---

Found by Sprint 537 W5 on noviwin1 (Windows 11 amd64, Go 1.27.0 at C:\Program Files\Go\bin\go), 2026-10-30. A command with an env-assignment prefix fails command lookup when it is a pipeline element; the same command in a subshell or bare works. Reproduces with scoop bashy 0.32.0-dev AND a bashy built from bashy main f3bbb8bc.
Repro script (run with bashy on Windows):
  GOWORK=off go version | cat          # -> line 1: go: command not found
  (GOWORK=off CGO_ENABLED=0 go version) # -> go version go1.27.0 windows/amd64
  go version | cat                     # (expected to work; confirm in the fix)
Expected: identical behaviour on every OS (macOS/Linux run all three). Likely the pipeline path resolves argv[0] with the prefix-assignment env (PATH/PATHEXT not carried) on Windows.
Red/green: a Windows-only (or seam-tested) unit test in sh/bashy that an assignment-prefixed command inside a pipeline resolves via PATH+PATHEXT.
Note: S537 desk launchers do not depend on this form (they use the subshell form), so this is not a desk blocker.
