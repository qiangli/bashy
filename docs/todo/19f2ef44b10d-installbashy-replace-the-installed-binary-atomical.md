---
id: 19f2ef44b10d
kind: bug
title: 'installbashy: replace the installed binary atomically and verified (temp file, --version check, rename)'
seq: 3224
status: todo
priority: p2
created: 2026-11-11T12:18:34.586291Z
---

Problem: overwriting the installed bashy binary in place (copy over the running file, or delete then copy) left an agent session whose shell IS that binary (CLAUDE_CODE_SHELL) unable to run any command (2026-11-11, Sprint 613); the in-place copy was Killed by macOS (overwrite of a signed, mapped binary). Wanted: tools/installbashy and any self-update write a temp file in the target directory, run it with --version, then rename(2) it over the target; on failure the old binary is untouched. Verification: installbashy unit test with a failing candidate leaves the target byte-identical; manual: install while a shell from the old binary runs, both keep working. The operator-side half (a harness hook) is already in place.
