---
id: 07edf60e1999
kind: task
title: One-time operator-approved migration of persistent bashy browser profiles off OS keychain encryption
seq: 3277
status: todo
priority: p2
labels:
    - browser
created: 2026-11-20T23:55:52.229972Z
---

Follow-up to e1b0b5ae: existing initialized persistent bashy-owned Chrome profiles (such as browser-probe-9222 containing Local State / Default) keep their current flags so cookies encrypted with the real OS keychain key are not silently invalidated. Design and offer an explicit one-time operator-approved migration command to re-initialize or re-authenticate persistent bashy browser profiles under --use-mock-keychain / --password-store=basic.
