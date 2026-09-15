---
id: 9d6684b3accf
kind: task
title: S194.7 verify awd concurrency through the installed bashy front door
seq: 293
status: assigned
priority: p1
created: 2026-09-15T12:05:21.579193Z
weave: 25
assignee: qiangli
sprint: 194
---

Add installed/front-door regression coverage for S194.6: concurrent Classic awd jobs and Bash++ go tasks must report branch-local directories and preserve the parent cwd/PWD/OLDPWD state. Consume the published sh fix or test-only contract via .sibling-pins. Gate: focused bashy e2e under -race where supported, CI=true make test, and installed stress loop.
