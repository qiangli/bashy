---
id: 1f1a22672062
kind: task
title: S194.4 convert Python TypeScript Rust dag smoke gates to pinned cache clones
seq: 291
status: todo
priority: p1
created: 2026-09-15T12:05:21.497402Z
sprint: 194
---

Adopt smoke-dag-c's URL+commit, shallow cache clone and operator *_ROOT override shape in scripts/dag-python-examples-smoke.sh, dag-typescript-examples-smoke.sh and dag-rust-examples-smoke.sh. Keep exact per-project pins visible and check byte-identical status. Gate: installed make smoke-dag-python smoke-dag-typescript smoke-dag-rust smoke-dag-c with all checkout-root env vars unset, cold then warm; explicit-root path remains green.
