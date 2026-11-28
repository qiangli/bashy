---
id: 24f6cdb09b79
kind: bug
title: 'TestToolShortcutE2E fails on a dev host with live claude shells: bashy chat sessions did not finish within 15s'
seq: 3337
status: todo
priority: p3
labels:
    - test
created: 2026-11-28T08:30:26.120147Z
---

Seen 2026-11-28 on bashy 1e15e238 and 13e33577 on the operator's macOS dev box (go1.27.4): toolshortcut_e2e_test.go:299, the chat sessions listing shows the host's live claude shell sessions and the command does not finish within 15s; the first run also downloaded modules inside the window. Passes on the Windows test-host lane (sprint 739). Likely reads the real session registry instead of the test's scratch HOME, or the 15s window is too short with a cold module cache.
