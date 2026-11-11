---
id: a847aaf17506
kind: feature
title: 'M2b examples/cast: cast record as a custom command (Go fence over pty/ConPTY), not core'
seq: 3218
status: assigned
priority: p1
labels:
    - media
    - windows
created: 2026-11-11T03:52:26.50563Z
assignee: claude-fable5.1-w89
sprint: 603
sprint_id: c808e55d-6ce2-5525-8829-96499861c90e
sprint_title: 'Agent-assisted PDF and video: small verbs, a media skill-script and kb pages any agent can follow'
---

Supersedes the core yoke delivery of M2 (yoke story b3941737b94c): operator decision 2026-11-10 — `cast` stays a custom command (example level) unless core is absolutely necessary; it is not (record.bsh already relies on a Go fence, so members already have the toolchain). The core commit was withdrawn before push; its code (the race winner by claude-fable5.1) is the starting point.

GOAL: `examples/cast/cast.bsh`, a Bash# skill-script (command + `~~~skill as guide`, same form as examples/record/record.bsh) with a `~~~go` fence that records asciicast v2 under the platform pseudo-terminal (creack/pty on Unix, aymanbagabas/go-pty ConPTY on Windows), with its own `examples/cast/go.mod` declaring those two dependencies. Interface unchanged: `cast record OUT.cast [--cols N] [--rows N] [--idle-limit S] [--title T] -- CMD [ARGS...]`, OUT may be `-`, exit status is CMD's; UTF-8 carry across reads; Windows ConPTY drain on exit. Register `cast` in examples/defaults DEFAULT_COMMANDS (effects read,write,exec). examples/video/video.sh already calls `cast record` (or `bashy cast record`); make that resolve to the registered command and make examples/video/test.sh's cast-path check run (not skip) when cast is registered.
TESTS: a test that records printf with colour codes and multi-byte UTF-8 split across reads, parses the cast (header width/height, monotonic times, bytes intact); GOOS=windows build of the fence package compiles; examples/video/test.sh cast path passes on this host.
