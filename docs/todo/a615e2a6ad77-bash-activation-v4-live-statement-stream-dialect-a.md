---
id: a615e2a6ad77
kind: task
title: 'Bash++ activation v4: live statement-stream dialect across every input mode'
seq: 196
status: done
priority: p0
created: 2026-09-03T11:15:58.314116Z
sprint: 98
closed: 2026-09-03T12:20:12.938709Z
---

Depends on pushed sh ef4ef743 live dialect seam. Replace rejected activation attempts. Wire --bashpp and runtime set -o/+o bashpp into -c, file, stdin, readline interactive, forced-interactive, and RunSessionCommand using a true statement-stream parser that selects syntax from Runner.Dialect/PosixMode before each statement, including semicolon-separated statements in one command. No bytes.Contains/source sniffing. Preserve synthetic/internal classic parses. POSIX composes and disables Bash++ grammar behavior. Update help, setopt/compgen vocabulary and .sibling-pins. Tests must execute real Bash++ syntax after live enable and reject/fallback after disable in every path; B4 compatibility contract must remain exact. Clean bootstrap and tests.
