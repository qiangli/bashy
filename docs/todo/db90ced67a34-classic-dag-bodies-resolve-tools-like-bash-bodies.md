---
id: db90ced67a34
kind: enhancement
title: 'Classic dag bodies resolve tools like Bash# bodies: builtin, bundled, bashy front door, registered, PATH last'
seq: 1937
status: todo
priority: p2
labels:
    - dag
    - release
created: 2026-10-17T15:14:20.725686Z
sprint: 407
sprint_id: 90546935-fbc2-5d09-94b0-19c380846681
sprint_title: bashy 1.0.0 release gates pending external dependencies (follow-up to Sprint 379)
---

Carved 2026-10-17 from post-1.0 story d4d97e6da1e5 (S367.1 part D, Sprint 403 closed). For v1.0.0: classic and Bash# dag bodies must not resolve tools differently, and the dag examples must not require build tools on PATH.

Scope (bashy + yoke)
- For classic (bash/sh/untagged) dag bodies, install the same exec lookup bsh bodies already get: builtin, bundled utility, bashy front door (go, cmake, cargo, uv, npm), registered command, then PATH last (no behaviour loss).
- Hook point: bashy/internal/agentos next to dag_bashpp.go; add an exec-handler seam in yoke/pkg/dag/interp_bash.go if missing (yoke stays tool-agnostic).
- Do NOT change bashy shell command lookup (it is correct; changing it regresses GNU compat and POSIX). Only the dag body exec handler.
- Drop the stale "must be on PATH" notes in examples/dag/README.md where a front door now covers the tool.

NOT this story: make/npm targets as fence methods (part A, still in d4d97e6da1e5, post-1.0); GNU make provisioning (S365.3).

Acceptance: red/green tests in the touched packages; with no cmake on PATH, a classic dag body running cmake --version succeeds through the front door; an unknown tool still falls through to PATH; make test unaffected.
