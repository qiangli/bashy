---
id: 3a934b30df02
kind: bug
title: install-agent codex --hooks --project writes the user-level ~/.codex/config.toml
seq: 417
status: todo
priority: p2
labels:
    - install-agent
created: 2026-10-09T04:01:23.775161Z
---

Found during Sprint 321 #1105 verification (2026-10-09): bashy install-agent codex --hooks --project --as X reported writing $HOME/.codex/config.toml, so --project is silently ignored for codex and a scratch probe hook landed in the operator's global Codex config (removed with --hooks --uninstall). Either honour --project (project .codex/config.toml if Codex supports it) or refuse --project for codex with a clear message. Red test first.
