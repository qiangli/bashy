---
id: 9b8ce84efc41
kind: bug
title: bashy satellite rm refuses --json ('--json unsupported for satellite') although the yoke verb has the flag
seq: 3239
status: done
priority: p2
labels:
    - fleet
created: 2026-11-14T22:43:01.718786Z
closed: 2026-11-15T01:54:56.489874Z
---

Resolved by analysis 2026-11-14: not a --json table defect. The bashy built from the siblings pinned yoke 2dfda687 (no 'satellite rm'); cobra resolved 'satellite rm --json' to the parent 'satellite' command, which has no --json, hence '--json unsupported for satellite'. With the yoke pin at d388f95d the verb exists and carries --json (verified: built bashy, 'satellite rm --help' lists --json).
