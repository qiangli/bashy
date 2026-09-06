---
id: 5eacb678ffdb
kind: task
title: dag build omits the native signal launcher, so dag install ships the payload as bashy
seq: 243
status: todo
priority: p1
created: 2026-09-06T11:24:04.827053Z
sprint: 115
---

DEFECT: `bashy dag build` and `make build` disagree on linux/darwin, and the
disagreement silently degrades a host install.

The Makefile's build-bash/build-bashy compile TWO artifacts per program there:

    go build -o bin/<name>.real ./cmd/<name>      # the Go payload
    cc -x c ... -o bin/<name> native/siglaunch.c.in   # native pre-Go launcher

DAG.md's build did a plain `go build -o bin/<name>`, producing no launcher and
no .real sidecar. tools/installbashy then installs what it is given, so
`bashy dag install` put the Go payload at ~/.local/bin/bashy and
~/.local/bin/bash.

MEASURED: after `bashy dag install`, both installed programs were the Go
binaries (bashy 94644306, bash 8117522) and the .real files were byte
duplicates of them. After `make install` the same paths are the 34232-byte
launcher plus the payload. The native signal launcher was simply absent from
the host install.

WHY IT IS SILENT: the payload runs. `bashy --version`, the bash drop-in and
every verb behave normally, so nothing fails visibly — what is missing is the
pre-Go signal handling the launcher exists to provide, which shows up only
under signals. installbashy's own comment states the contract ("a native pre-Go
signal launcher plus a sibling Go payload") and its surface check does not
cover it, because the payload passes that check.

FIX (applied): DAG.md's build now mirrors the Makefile, plus a guard the
Makefile does not need. The launcher is compiled by the HOST cc, so it can only
be produced for the host's own platform; DAG's build accepts a GOOS override
for cross-builds, and pairing a host-native launcher with a foreign payload
would be a broken pair that builds cleanly and fails only when run. So the
split applies when the target is linux/darwin AND equals the host, and a
cross-build emits the plain Go binaries.

Also brought over: `scripts/build-meet-spa.sh optional`, which the Makefile ran
and DAG.md did not.
