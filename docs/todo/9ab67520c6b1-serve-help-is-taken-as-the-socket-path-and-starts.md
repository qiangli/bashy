---
id: 9ab67520c6b1
kind: bug
title: 'serve: ''--help'' is taken as the socket path and starts the server'
seq: 3313
status: todo
priority: p3
labels:
    - cli
created: 2026-11-25T10:37:47.333399Z
---

Symptom (2026-11-25, dev box): `bashy serve --help` printed nothing and ran until the 120 s tool timeout (stopped by hand).

Root cause (code inspected): internal/agentos/agentos.go, `case "serve"`: os.Args[2] is passed unvalidated as the socket path to session.Serve, so `--help` (or any flag) becomes a socket named '--help' and the warm-session server starts in the foreground.

Classification: product defect (argument handling). Fix: treat -h/--help (and any other leading '-') as help or a usage error before calling session.Serve; print the usage, exit 0 for help, 2 otherwise.

Verify: `bashy serve --help` prints the usage and exits at once; `bashy serve SOCKET` is unchanged; a unit test covers both.
