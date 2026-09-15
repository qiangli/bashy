---
id: 780944a84d33
kind: task
title: S194.5 add a pinned mise Rust-fence dag.md example
seq: 292
status: done
priority: p1
created: 2026-09-15T12:05:21.525244Z
weave: 24
assignee: qiangli
sprint: 194
closed: 2026-09-15T16:25:13.553236Z
closed_by: codex-gpt5.6-sol
---

Add examples/dag/mise/dag.md for jdx/mise pinned at 55d3b4fc789d76fbaa486cb523f92cc974ce67c7. Use the repo's own light test/build front door per AGENTS.md and a ~~~rs as rs smoke that reads checkout-owned version/workspace coordinates; add MISE_ROOT override + pin/clone integration to smoke-dag-rust and update example docs. Never modify the operator checkout; MBX_DISABLE=1 is only a surfaced fallback. Gate: explicit-root and zero-env Rust smoke, byte-identical git status.
